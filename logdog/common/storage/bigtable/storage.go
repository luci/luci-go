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
	"bytes"
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/bigtable"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/data/recordio"
	log "go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
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

var (
	// errStop is an internal sentinel error used to indicate "stop iteration"
	// to btTable.getLogData iterator.
	errStop = errors.New("bigtable: stop iteration")
)

// Storage is a BigTable storage configuration client.
type Storage struct {
	// Client, if not nil, is the BigTable client to use for BigTable accesses.
	Client *bigtable.Client

	// LogTable is the name of the BigTable table to use for logs.
	LogTable string

	// Cache, if not nil, will be used to cache data.
	Cache storage.Cache

	// testBTInterface, if not nil, is the BigTable interface to use. This is
	// useful for testing. If nil, this will default to the production instance.
	testBTInterface btIface
}

func (s *Storage) getIface() btIface {
	if s.testBTInterface != nil {
		return s.testBTInterface
	}
	return prodBTIface{s}
}

// Close implements storage.Storage.
func (s *Storage) Close() {}

// Put implements storage.Storage.
func (s *Storage) Put(c context.Context, r storage.PutRequest) error {
	c = prepareContext(c)

	iface := s.getIface()
	rw := rowWriter{
		threshold: iface.getMaxRowSize(),
	}

	for len(r.Values) > 0 {
		// Add the next entry to the writer.
		if appended := rw.append(r.Values[0]); !appended {
			// We have failed to append our maximum BigTable row size. Flush any
			// currently-buffered row data and try again with an empty buffer.
			count, err := rw.flush(c, iface, r.Index, r.Project, r.Path)
			if err != nil {
				return err
			}

			if count == 0 {
				// Nothing was buffered, but we still couldn't append an entry. The
				// current entry is too large by itself, so we must fail.
				return fmt.Errorf("single row entry exceeds maximum size (%d > %d)", len(r.Values[0]), rw.threshold)
			}

			r.Index += types.MessageIndex(count)
			continue
		}

		// We successfully appended this entry, so advance.
		r.Values = r.Values[1:]
	}

	// Flush any buffered rows.
	if _, err := rw.flush(c, iface, r.Index, r.Project, r.Path); err != nil {
		return err
	}
	return nil
}

// Expunge implements storage.Storage.
func (s *Storage) Expunge(c context.Context, r storage.ExpungeRequest) error {
	return s.getIface().dropRowRange(
		prepareContext(c), newRowKey(string(r.Project), string(r.Path), 0, 0))
}

// Get implements storage.Storage.
func (s *Storage) Get(c context.Context, r storage.GetRequest, cb storage.GetCallback) error {
	c = prepareContext(c)

	startKey := newRowKey(string(r.Project), string(r.Path), int64(r.Index), 0)
	c = log.SetFields(c, log.Fields{
		"project":     r.Project,
		"path":        r.Path,
		"index":       r.Index,
		"limit":       r.Limit,
		"startRowKey": startKey,
		"keysOnly":    r.KeysOnly,
	})

	// If we issue a query and get back a legacy row, it will have no count
	// associated with it. We will fast-exit

	limit := r.Limit
	err := s.getIface().getLogData(c, startKey, r.Limit, r.KeysOnly, func(rk *rowKey, data []byte) error {
		// Does this key match our requested log stream? If not, we've moved past
		// this stream's records and must stop iteration.
		if !rk.sharesPathWith(startKey) {
			return errStop
		}

		// Calculate the start index of the contiguous row. Since we index the row
		// on the LAST entry in the row, count backwards to get the index of the
		// first entry.
		startIndex := rk.firstIndex()
		if startIndex < 0 {
			return storage.ErrBadData
		}

		// Split our data into records. Leave the records slice nil if we're doing
		// a keys-only get.
		var records [][]byte
		if !r.KeysOnly {
			var err error
			if records, err = recordio.Split(data); err != nil {
				return storage.ErrBadData
			}

			if rk.count != int64(len(records)) {
				log.Fields{
					"count":       rk.count,
					"recordCount": len(records),
				}.Errorf(c, "Record count doesn't match declared count.")
				return storage.ErrBadData
			}
		}

		// If we are indexed somewhere within this entry's records, discard any
		// records before our index.
		if discard := int64(r.Index) - startIndex; discard > 0 {
			if discard > rk.count {
				// This should never happen unless there is corrupt or conflicting data.
				return nil
			}
			startIndex += discard
			if !r.KeysOnly {
				records = records[discard:]
			}
		}

		for index := startIndex; index <= rk.index; index++ {
			// If we're not doing keys-only, consume the row.
			var row []byte
			if !r.KeysOnly {
				row, records = records[0], records[1:]
			}

			if !cb(storage.MakeEntry(row, types.MessageIndex(index))) {
				return errStop
			}
			r.Index = types.MessageIndex(index + 1)

			// Artificially apply limit within our row records.
			if limit > 0 {
				limit--
				if limit == 0 {
					return errStop
				}
			}
		}
		return nil
	})

	switch err {
	case nil, errStop:
		return nil

	default:
		log.WithError(err).Errorf(c, "Failed to retrieve row range.")
		return err
	}
}

// Tail implements storage.Storage.
func (s *Storage) Tail(c context.Context, project string, path types.StreamPath) (*storage.Entry, error) {
	c = prepareContext(c)
	c = log.SetFields(c, log.Fields{
		"project": project,
		"path":    path,
	})
	iface := s.getIface()

	// Load the "last tail index" from cache. If we have no cache, start at 0.
	var startIdx int64
	if s.Cache != nil {
		startIdx = getLastTailIndex(c, s.Cache, project, path)
	}

	// Iterate through all log keys in the stream. Record the latest contiguous
	// one.
	var (
		rk        = newRowKey(project, string(path), startIdx, 0)
		latest    *rowKey
		nextIndex = startIdx
	)
	err := iface.getLogData(c, rk, 0, true, func(rk *rowKey, data []byte) error {
		// If this record is non-contiguous, we're done iterating.
		if rk.firstIndex() != nextIndex {
			return errStop
		}

		latest, nextIndex = rk, rk.index+1
		return nil
	})
	if err != nil && err != errStop {
		log.Fields{
			log.ErrorKey: err,
			"table":      s.LogTable,
		}.Errorf(c, "Failed to scan for tail.")
	}

	if latest == nil {
		// No rows for the specified stream.
		return nil, storage.ErrDoesNotExist
	}

	// Update our cache if the tail index has changed.
	if s.Cache != nil && startIdx != latest.index {
		// We cache the first index in the row so that subsequent cached fetches
		// have the correct "startIdx" expectations.
		putLastTailIndex(c, s.Cache, project, path, latest.firstIndex())
	}

	// Fetch the latest row's data.
	var d []byte
	err = iface.getLogData(c, latest, 1, false, func(rk *rowKey, data []byte) error {
		records, err := recordio.Split(data)
		if err != nil || len(records) == 0 {
			return storage.ErrBadData
		}
		d = records[len(records)-1]
		return errStop
	})
	if err != nil && err != errStop {
		log.Fields{
			log.ErrorKey: err,
			"table":      s.LogTable,
		}.Errorf(c, "Failed to retrieve tail row.")
	}

	return storage.MakeEntry(d, types.MessageIndex(latest.index)), nil
}

// rowWriter facilitates writing several consecutive data values to a single
// BigTable row.
type rowWriter struct {
	// buf is the current set of buffered data.
	buf bytes.Buffer

	// count is the number of rows in the writer.
	count int

	// threshold is the maximum number of bytes that we can write.
	threshold int
}

func (w *rowWriter) append(d []byte) (appended bool) {
	origSize := w.buf.Len()
	defer func() {
		// Restore our previous buffer state if we are reporting the write as
		// failed.
		if !appended {
			w.buf.Truncate(origSize)
		}
	}()

	// Serialize the next entry as a recordio blob.
	if _, err := recordio.WriteFrame(&w.buf, d); err != nil {
		return
	}

	// If we have exceeded our threshold, report a failure.
	appended = (w.buf.Len() <= w.threshold)
	if appended {
		w.count++
	}
	return
}

func (w *rowWriter) flush(c context.Context, iface btIface, index types.MessageIndex,
	project string, path types.StreamPath) (int, error) {

	flushCount := w.count
	if flushCount == 0 {
		return 0, nil
	}

	// Write the current set of buffered rows to the table. Index on the LAST
	// row index.
	lastIndex := int64(index) + int64(flushCount) - 1
	rk := newRowKey(string(project), string(path), lastIndex, int64(w.count))

	if err := iface.putLogData(c, rk, w.buf.Bytes()); err != nil {
		return 0, err
	}

	// Reset our buffer state.
	w.buf.Reset()
	w.count = 0
	return flushCount, nil
}

func prepareContext(c context.Context) context.Context {
	// Explicitly clear gRPC metadata from the Context. It is forwarded to
	// delegate calls by default, and standard request metadata can break BigTable
	// calls.
	return metadata.NewOutgoingContext(c, nil)
}
