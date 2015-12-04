// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"fmt"

	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	"google.golang.org/cloud/bigtable"
)

var (
	// StorageScopes is the set of OAuth scopes needed to use the storage
	// functionality.
	StorageScopes = []string{
		bigtable.Scope,
	}

	// StorageReadOnlyScopes is the set of OAuth scopes needed to use the storage
	// functionality.
	StorageReadOnlyScopes = []string{
		bigtable.ReadonlyScope,
	}
)

const (
	// tailRowCount is the size of the block of rows that tail read operations
	// pull from BigTable. This is designed to be large enough for efficient
	// buffering while staying small enough to avoid wasteful reads or
	// excessive in-memory buffering.
	//
	// This is simply the maximum number of rows (limit). The actual number of
	// rows will be further constrained by tailRowMaxSize.
	tailRowCount = 128

	// tailRowMaxSize is the maximum number of bytes of tail row data that will be
	// buffered during Tail row reading.
	tailRowMaxSize = 1024 * 1024 * 16
)

// Options is a set of configuration options for BigTable storage.
type Options struct {
	// Project is the name of the project to connect to.
	Project string
	// Zone is the name of the zone to connect to.
	Zone string
	// Cluster is the name of the cluster to connect to.
	Cluster string
	// ClientOptions are additional client options to use when instantiating the
	// client instance.
	ClientOptions []cloud.ClientOption

	// Table is the name of the BigTable table to use for logs.
	LogTable string

	// EnableGarbageCollection is used during initialization only. If true, it
	// will enable pushing garbage collection settings to BigTable.
	//
	// At the time of writing, attempts to use garbage collection configuration
	// methods result in an error from the server stating that the methods are not
	// yet available.
	//
	// TODO(dnj): Remove this and default to true as soon as garbage collection
	// is enabled.
	EnableGarbageCollection bool
}

// btStorage is a storage.Storage implementation that uses Google Cloud BigTable
// as a backend.
type btStorage struct {
	*Options

	ctx    context.Context
	client *bigtable.Client

	table btTable
}

func streamRecordRowKey(sr *storage.StreamRecord) *rowKey {
	return newRowKey(string(sr.Path), int64(sr.Index))
}

// New instantiates a new Storage instance connected to a BigTable cluster.
//
// The returned Storage instance will close the Client when its Close() method
// is called.
func New(ctx context.Context, o Options) (storage.Storage, error) {
	c, err := bigtable.NewClient(ctx, o.Project, o.Zone, o.Cluster, o.ClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %s", err)
	}

	return &btStorage{
		Options: &o,
		ctx:     ctx,
		client:  c,
		table:   &btTableProd{c.Open(o.LogTable)},
	}, nil
}

func (s *btStorage) Close() {
	s.client.Close()
}

func (s *btStorage) Put(r *storage.PutRequest) error {
	rk := streamRecordRowKey(&r.StreamRecord)
	ctx := log.SetFields(s.ctx, log.Fields{
		"rowKey": rk,
		"path":   r.Path,
		"index":  r.Index,
		"size":   len(r.Value),
	})
	log.Debugf(ctx, "Adding entry to BigTable.")

	return s.table.putLogData(ctx, rk, r.Value)
}

func (s *btStorage) Get(r *storage.GetRequest, cb storage.GetCallback) error {
	startKey := streamRecordRowKey(&r.StreamRecord)
	c := log.SetFields(s.ctx, log.Fields{
		"path":        r.Path,
		"index":       r.Index,
		"startRowKey": startKey,
	})

	err := s.table.getLogData(c, startKey, r.Limit, false, func(rk *rowKey, data []byte) error {
		// Does this key match our requested log stream? If not, we've moved past
		// this stream's records and must stop iteration.
		if !rk.sharesPathWith(startKey) {
			return errStop
		}

		// We have a row. Invoke our callback.
		if !cb(types.MessageIndex(rk.index), data) {
			return errStop
		}
		return nil
	})
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    s.Project,
			"zone":       s.Zone,
			"cluster":    s.Cluster,
			"table":      s.LogTable,
		}.Errorf(c, "Failed to retrieve row range.")
		return err
	}
	return nil
}

func (s *btStorage) Tail(r *storage.GetRequest, cb storage.GetCallback) error {
	c := log.SetFields(s.ctx, log.Fields{
		"path":  r.Path,
		"index": r.Index,
	})

	// The current strategy for implementing Tail works in multiple passes:
	//
	// First Pass: SCAN
	// Scan forwards (keys-only) from the starting key looking for the last
	// contiguous index.
	ridx, err := s.getLastContiguousIndex(c, &r.StreamRecord, r.Limit)
	if err != nil {
		return err
	}

	if ridx < int64(r.Index) {
		return storage.ErrDoesNotExist
	}
	count := ridx - int64(r.Index) + 1

	// Second pass: EMIT
	// Read rows in fixed-size chunks starting from the end. Punt them in reverse
	// order to the callback.
	rowCount := tailRowCount
	if int64(rowCount) > count {
		rowCount = int(count)
	}
	rows := make([][]byte, 0, rowCount)

	greq := storage.GetRequest{
		StreamRecord: r.StreamRecord,
	}

	for count > 0 {
		// How many records to pull this round?
		amount := int64(cap(rows))
		if amount > count {
			amount = count
		}

		// Pull the marked rows for export. We don't have to do any specific checks
		// here since the rows have already been vetted.
		rows = rows[:0]
		greq.Index = types.MessageIndex(ridx - amount + 1)
		greq.Limit = int(amount)
		rk := streamRecordRowKey(&r.StreamRecord)
		err := s.table.getLogData(c, rk, greq.Limit, false, func(rk *rowKey, data []byte) error {
			rows = append(rows, data)
			return nil
		})
		if err != nil {
			return err
		}

		// Issue our callbacks in reverse.
		for i := 0; i < len(rows); i++ {
			if !cb(types.MessageIndex(ridx-int64(i)), rows[len(rows)-i-1]) {
				return nil
			}
		}
		count -= amount
	}
	return nil
}

// getLastContiguousIndex returns the stream index of the last contiguous
// LogEntry in the specified stream.
//
// Currently, this operates by iterating forwards from the start index and
// identifying the first row that has a different stream path or a
// non-contiguous index.
//
// This is fairly cheap, but certainly more costly than is necessary. A few
// optimizations are possible:
// - Stream rows can be stored in a parallel reverse table making the search
//   trivial.
// - The last contiguous row for a given stream could be cached in a separate
//   table. Future searches would start with that index. This would avoid
//   traversing the massive row space more times than necessary.
func (s *btStorage) getLastContiguousIndex(c context.Context, r *storage.StreamRecord, limit int) (int64, error) {
	lastIndex := int64(r.Index) - 1
	startKey := streamRecordRowKey(r)
	err := s.table.getLogData(c, startKey, limit, true, func(rk *rowKey, data []byte) error {
		if !rk.sharesPathWith(startKey) {
			return errStop
		}

		if rk.index != lastIndex+1 {
			// We have encountered a non-contiguous row.
			return errStop
		}

		lastIndex = rk.index
		return nil
	})
	if err != nil {
		return -1, err
	}
	return lastIndex, nil
}

func (s *btStorage) Purge(r *storage.StreamRecord) error {
	panic("Not implemented.")
}

// getLogData loads the "data" column from the "log" column family, Unmarshals
// it into a LogRecord, and
// it to
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
