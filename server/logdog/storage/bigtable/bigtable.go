// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"fmt"
	"strings"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
	"google.golang.org/cloud/bigtable"
)

// errStop is an internal sentinel error used to communicate "stop iteration"
// to btTable.getLogData.
var errStop = errors.New("stop")

// btGetCallback is a callback that is invoked for each log data row returned
// by getLogData.
//
// If an error is encountered, no more log data will be fetched. The error will
// be propagated to the getLogData call unless the returned error is errStop, in
// which case iteration will stop and getLogData will return nil.
type btGetCallback func(*rowKey, []byte) error

// btTable is a general interface for BigTable operations intended to enable
// unit tests to stub out BigTable without adding runtime inefficiency.
//
// If any of these methods fails with a transient error, it will be wrapped
// as an errors.Transient error.
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
}

// btTransientSubstrings is the set of known error substrings returned by
// BigTable that indicate failures that aren't related to the specific data
// content.
var btTransientSubstrings = []string{
	"Internal error encountered",
	"interactive login is required",
}

// btTableProd is an implementation of the btTable interface that uses a real
// production BigTable connection.
type btTableProd struct {
	*bigtable.Table
}

func (t *btTableProd) putLogData(c context.Context, rk *rowKey, data []byte) error {
	m := bigtable.NewMutation()
	m.Set("log", "data", bigtable.ServerTime, data)
	cm := bigtable.NewCondMutation(bigtable.RowKeyFilter(rk.encode()), nil, m)

	rowExists := false
	err := t.Apply(c, rk.encode(), cm, bigtable.GetCondMutationResult(&rowExists))
	if err != nil {
		return wrapTransient(err)
	}
	if rowExists {
		return storage.ErrExists
	}
	return nil
}

func (t *btTableProd) getLogData(c context.Context, rk *rowKey, limit int, keysOnly bool, cb btGetCallback) error {
	// Construct read options based on Get request.
	ropts := []bigtable.ReadOption{
		bigtable.RowFilter(bigtable.FamilyFilter("log")),
		bigtable.RowFilter(bigtable.ColumnFilter("data")),
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
	err := t.ReadRows(c, rng, func(row bigtable.Row) bool {
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
//
// TODO(dnj): File issue to add error qualifier functions to BigTable API.
func isTransient(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	for _, s := range btTransientSubstrings {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// getLogData loads the "data" column from the "log" column family and returns
// its []byte contents.
//
// If the row doesn't exist, storage.ErrDoesNotExist will be returned.
func getLogData(row bigtable.Row) ([]byte, error) {
	ri := getReadItem(row, "log", "data")
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
