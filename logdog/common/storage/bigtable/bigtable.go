// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigtable

import (
	"context"
	"fmt"

	"go.chromium.org/luci/logdog/common/storage"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"cloud.google.com/go/bigtable"
	"google.golang.org/grpc/codes"
)

const (
	logColumnFamily = "log"

	// The data column stores raw low row data (RecordIO blob).
	logColumn  = "data"
	logColName = logColumnFamily + ":" + logColumn
)

// Limits taken from here:
// https://cloud.google.com/bigtable/docs/schema-design
const (
	// bigTableRowMaxBytes is the maximum number of bytes that a single BigTable
	// row may hold.
	bigTableRowMaxBytes = 1024 * 1024 * 10 // 10MB
)

// btGetCallback is a callback that is invoked for each log data row returned
// by getLogData.
//
// If an error is encountered, no more log data will be fetched. The error will
// be propagated to the getLogData call.
type btGetCallback func(*rowKey, []byte) error

// btIface is a general interface for BigTable operations intended to enable
// unit tests to stub out BigTable without adding runtime inefficiency.
type btIface interface {
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

	// Drops all rows given the path prefix of rk.
	dropRowRange(c context.Context, rkPrefix *rowKey) error

	// getMaxRowSize returns the maximum row size that this implementation
	// supports.
	getMaxRowSize() int
}

// prodBTIface is a production implementation of a "btIface".
type prodBTIface struct {
	*Storage
}

func (bti prodBTIface) getLogTable() (*bigtable.Table, error) {
	if bti.Client == nil {
		return nil, errors.New("no client configured")
	}
	return bti.Client.Open(bti.LogTable), nil
}

func (bti prodBTIface) putLogData(c context.Context, rk *rowKey, data []byte) error {
	logTable, err := bti.getLogTable()
	if err != nil {
		return err
	}

	m := bigtable.NewMutation()
	m.Set(logColumnFamily, logColumn, bigtable.ServerTime, data)
	cm := bigtable.NewCondMutation(bigtable.RowKeyFilter(rk.encode()), nil, m)

	rowExists := false
	if err := logTable.Apply(c, rk.encode(), cm, bigtable.GetCondMutationResult(&rowExists)); err != nil {
		return wrapIfTransientForApply(err)
	}
	if rowExists {
		return storage.ErrExists
	}
	return nil
}

func (bti prodBTIface) dropRowRange(c context.Context, rk *rowKey) error {
	logTable, err := bti.getLogTable()
	if err != nil {
		return err
	}

	// ApplyBulk claims to be able to apply 100k mutations. Keep it small here to
	// stay well within the stated guidelines.
	const maxBatchSize = 100000 / 4

	del := bigtable.NewMutation()
	del.DeleteRow()

	allMuts := make([]*bigtable.Mutation, maxBatchSize)
	for i := range allMuts {
		allMuts[i] = del
	}

	prefix, upperBound := rk.pathPrefix(), rk.pathPrefixUpperBound()
	rng := bigtable.NewRange(prefix, upperBound)
	// apply paranoia mode
	if rng.Contains("") || prefix == "" || upperBound == "" {
		panic(fmt.Sprintf("NOTHING MAKES SENSE: %q %q %q", rng, prefix, upperBound))
	}

	keyC := make(chan string)

	// TODO(iannucci): parallelize row scan?

	// buffered to avoid deadlocking main thread below
	readerC := make(chan error, 1)
	go func() {
		defer close(readerC)
		defer close(keyC)
		readerC <- logTable.ReadRows(c, rng, func(row bigtable.Row) bool {
			keyC <- row.Key()
			return true
		},
			bigtable.RowFilter(bigtable.FamilyFilter(logColumnFamily)),
			bigtable.RowFilter(bigtable.ColumnFilter(logColumn)),
			bigtable.RowFilter(bigtable.StripValueFilter()),
		)
	}()

	keys := make([]string, maxBatchSize)
	batchNum := 0
	var totalDropped int64
	for {
		batchNum++
		batch := keys[:0]
		for key := range keyC {
			batch = append(batch, key)
			if len(batch) >= maxBatchSize {
				break
			}
		}
		if len(batch) == 0 {
			logging.Infof(c, "dropRowRange: dropped %d rows", totalDropped)
			err, _ := <-readerC
			return err
		}

		logging.Infof(c, "dropRowRange: dropping %d rows", len(batch))
		errs, err := logTable.ApplyBulk(c, batch, allMuts[:len(batch)])
		if err != nil {
			logging.WithError(err).Errorf(c, "dropRowRange: ApplyBulk failed")
			return errors.Annotate(err, "ApplyBulk failed on batch %d", batchNum).Err()
		}
		if len(errs) > 0 {
			logging.Warningf(c, "ApplyBulk: got %d errors: first: %q", len(errs), errs[0])
		}
		totalDropped += int64(len(batch) - len(errs))
	}
}

func (bti prodBTIface) getLogData(c context.Context, rk *rowKey, limit int, keysOnly bool, cb btGetCallback) error {
	logTable, err := bti.getLogTable()
	if err != nil {
		return err
	}

	// Construct read options based on Get request.
	ropts := []bigtable.ReadOption{
		bigtable.RowFilter(bigtable.FamilyFilter(logColumnFamily)),
		bigtable.RowFilter(bigtable.ColumnFilter(logColumn)),
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

	var innerErr error
	err = logTable.ReadRows(c, rng, func(row bigtable.Row) bool {
		data, err := getLogRowData(row)
		if err != nil {
			innerErr = storage.ErrBadData
			return false
		}

		drk, err := decodeRowKey(row.Key())
		if err != nil {
			innerErr = err
			return false
		}

		if err := cb(drk, data); err != nil {
			innerErr = err
			return false
		}
		return true
	}, ropts...)
	if err != nil {
		return grpcutil.WrapIfTransient(err)
	}
	if innerErr != nil {
		return innerErr
	}
	return nil
}

func (bti prodBTIface) getMaxRowSize() int { return bigTableRowMaxBytes }

// getLogRowData loads the []byte contents of the supplied log row.
//
// If the row doesn't exist, storage.ErrDoesNotExist will be returned.
func getLogRowData(row bigtable.Row) (data []byte, err error) {
	items, ok := row[logColumnFamily]
	if !ok {
		err = storage.ErrDoesNotExist
		return
	}

	for _, item := range items {
		switch item.Column {
		case logColName:
			data = item.Value
			return
		}
	}

	// If no fields could be extracted, the rows does not exist.
	err = storage.ErrDoesNotExist
	return
}

// getReadItem retrieves a specific RowItem from the supplied Row.
func getReadItem(row bigtable.Row, family, column string) *bigtable.ReadItem {
	// Get the row for our family.
	items, ok := row[logColumnFamily]
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

func wrapIfTransientForApply(err error) error {
	if err == nil {
		return nil
	}

	// For Apply, assume that anything other than InvalidArgument (bad data) is
	// transient. We exempt InvalidArgument because our data construction is
	// deterministic, and so this request can never succeed.
	switch code := grpcutil.Code(err); code {
	case codes.InvalidArgument:
		return err
	default:
		return transient.Tag.Apply(err)
	}
}
