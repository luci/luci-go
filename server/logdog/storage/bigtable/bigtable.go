// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"fmt"
	"time"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/grpcutil"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
	"google.golang.org/cloud/bigtable"
)

// errStop is an internal sentinel error used to communicate "stop iteration"
// to btTable.getLogData.
var errStop = errors.New("stop")

const (
	logColumnFamily = "log"
	logColumn       = "data"
)

// btGetCallback is a callback that is invoked for each log data row returned
// by getLogData.
//
// If an error is encountered, no more log data will be fetched. The error will
// be propagated to the getLogData call unless the returned error is errStop, in
// which case iteration will stop and getLogData will return nil.
type btGetCallback func(*rowKey, []byte) error

// btTable is a general interface for BigTable operations intended to enable
// unit tests to stub out BigTable without adding runtime inefficiency.
type btTable interface {
	// putLogData adds new log data to BigTable.
	//
	// If data already exists for the named row, it will return storage.ErrExists
	// and not add the data.
	putLogData(context.Context, *rowKey, []byte) error

	// getLogData retrieves rows belonging to the supplied stream record, starting
	// with the first index owned by that record. The supplied callback is invoked
	// once per retrieved row.
	//
	// rk is the starting row key.
	//
	// If the supplied limit is nonzero, no more than limit rows will be
	// retrieved.
	//
	// If keysOnly is true, then the callback will return nil row data.
	getLogData(c context.Context, rk *rowKey, limit int, keysOnly bool, cb btGetCallback) error

	// setMaxLogAge updates the maximum log age policy for the log family.
	setMaxLogAge(context.Context, time.Duration) error
}

// btTableProd is an implementation of the btTable interface that uses a real
// production BigTable connection.
type btTableProd struct {
	*btStorage
}

func (t *btTableProd) putLogData(c context.Context, rk *rowKey, data []byte) error {
	table, err := t.getLogTable()
	if err != nil {
		return err
	}

	m := bigtable.NewMutation()
	m.Set(logColumnFamily, logColumn, bigtable.ServerTime, data)
	cm := bigtable.NewCondMutation(bigtable.RowKeyFilter(rk.encode()), nil, m)

	rowExists := false
	if err = table.Apply(c, rk.encode(), cm, bigtable.GetCondMutationResult(&rowExists)); err != nil {
		return wrapTransient(err)
	}
	if rowExists {
		return storage.ErrExists
	}
	return nil
}

func (t *btTableProd) getLogData(c context.Context, rk *rowKey, limit int, keysOnly bool, cb btGetCallback) error {
	table, err := t.getLogTable()
	if err != nil {
		return err
	}
	// Construct read options based on Get request.
	ropts := []bigtable.ReadOption{
		bigtable.RowFilter(bigtable.FamilyFilter(logColumnFamily)),
		bigtable.RowFilter(bigtable.ColumnFilter(logColumn)),
		nil,
		nil,
	}[:2]
	if keysOnly {
		ropts = append(ropts, bigtable.RowFilter(bigtable.StripValueFilter()))
	}
	if limit > 0 {
		ropts = append(ropts, bigtable.LimitRows(int64(limit)))
	}

	// This will limit the range to the immediate row key ("ASDF~INDEX") to
	// immediately after the row key ("ASDF~~"). See rowKey for more information.
	rng := bigtable.NewRange(rk.encode(), rk.pathPrefixUpperBound())

	innerErr := error(nil)
	err = table.ReadRows(c, rng, func(row bigtable.Row) bool {
		data := []byte(nil)
		if !keysOnly {
			err := error(nil)
			data, err = getLogData(row)
			if err != nil {
				innerErr = storage.ErrBadData
				return false
			}
		}

		drk, err := decodeRowKey(row.Key())
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"value":      row.Key(),
			}.Warningf(c, "Failed to parse row key.")
			innerErr = storage.ErrBadData
			return false
		}

		if err := cb(drk, data); err != nil {
			if err != errStop {
				innerErr = err
			}
			return false
		}
		return true
	}, ropts...)
	if err == nil {
		err = innerErr
	}
	return wrapTransient(err)
}

func (t *btTableProd) setMaxLogAge(c context.Context, d time.Duration) error {
	ac, err := t.getAdminClient()
	if err != nil {
		return err
	}

	var logGCPolicy bigtable.GCPolicy
	if d > 0 {
		logGCPolicy = bigtable.MaxAgePolicy(d)
	}
	if err := ac.SetGCPolicy(c, t.LogTable, logColumnFamily, logGCPolicy); err != nil {
		return wrapTransient(err)
	}
	return nil
}

// wrapTransient wraps the supplied error in an errors.TransientError if it is
// transient.
func wrapTransient(err error) error {
	if isTransient(err) {
		err = errors.WrapTransient(err)
	}
	return err
}

// isTransient tests if a BigTable SDK error is transient.
//
// Since the BigTable API doesn't give us this information, we will identify
// transient errors by parsing their error string :(
func isTransient(err error) bool {
	return grpcutil.IsTransient(err)
}

// getLogData loads the logColumn column from the logColumnFamily column family
// and returns its []byte contents.
//
// If the row doesn't exist, storage.ErrDoesNotExist will be returned.
func getLogData(row bigtable.Row) ([]byte, error) {
	ri := getReadItem(row, logColumnFamily, logColumn)
	if ri == nil {
		return nil, storage.ErrDoesNotExist
	}
	return ri.Value, nil
}

// getReadItem retrieves a specific RowItem from the supplied Row.
func getReadItem(row bigtable.Row, family, column string) *bigtable.ReadItem {
	// Get the row for our family.
	items, ok := row[family]
	if !ok {
		return nil
	}

	// Get the specific ReadItem for our column
	colName := fmt.Sprintf("%s:%s", family, column)
	for _, item := range items {
		if item.Column == colName {
			return &item
		}
	}
	return nil
}
