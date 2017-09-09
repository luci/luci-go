// Copyright 2016 The LUCI Authors.
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

package datastore

import (
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"

	"golang.org/x/net/context"
)

func applyBatchFilter(c context.Context, rds RawInterface) RawInterface {
	batchingEnabled, batchingSpecified := getBatching(c)
	return &batchFilter{
		RawInterface:      rds,
		ic:                c,
		constraints:       rds.Constraints(),
		batchingSpecified: batchingSpecified,
		batchingEnabled:   batchingEnabled,
	}
}

type batchFilter struct {
	RawInterface

	ic                context.Context
	constraints       Constraints
	batchingSpecified bool
	batchingEnabled   bool
}

func (bf *batchFilter) GetMulti(keys []*Key, meta MultiMetaGetter, cb GetMultiCB) error {
	return bf.batchParallel(len(keys), bf.constraints.MaxGetSize, func(offset, count int) error {
		return bf.RawInterface.GetMulti(keys[offset:offset+count], meta, func(idx int, val PropertyMap, err error) error {
			return cb(offset+idx, val, err)
		})
	})
}

func (bf *batchFilter) PutMulti(keys []*Key, vals []PropertyMap, cb NewKeyCB) error {
	return bf.batchParallel(len(vals), bf.constraints.MaxPutSize, func(offset, count int) error {
		return bf.RawInterface.PutMulti(keys[offset:offset+count], vals[offset:offset+count], func(idx int, key *Key, err error) error {
			return cb(offset+idx, key, err)
		})
	})
}

func (bf *batchFilter) DeleteMulti(keys []*Key, cb DeleteMultiCB) error {
	return bf.batchParallel(len(keys), bf.constraints.MaxDeleteSize, func(offset, count int) error {
		return bf.RawInterface.DeleteMulti(keys[offset:offset+count], func(idx int, err error) error {
			return cb(offset+idx, err)
		})
	})
}

func (bf *batchFilter) batchParallel(count, batch int, cb func(offset, count int) error) error {
	// If no batch size is defined, or if this can be done in one batch, then do
	// everything in a single batch.
	if batch <= 0 || count <= batch {
		return cb(0, count)
	}

	// We batch by default unless the user specifies otherwise.
	batching := (bf.batchingEnabled || !bf.batchingSpecified) && batch > 0

	// If batching is disabled, we will skip goroutines and do everything in a
	// single batch.
	if !batching {
		if batch > 0 && count > batch {
			return errors.Reason("batching is disabled, and size (%d) exceeds maximum (%d)", count, batch).Err()
		}
		return cb(0, count)
	}

	// Dispatch our batches in parallel.
	err := parallel.FanOutIn(func(workC chan<- func() error) {
		for i := 0; i < count; {
			offset := i
			size := count - i
			if size > batch {
				size = batch
			}

			workC <- func() error {
				return filterStop(cb(offset, size))
			}

			i += size
		}
	})

	// If our Context timed out or was cancelled, forward that error instead
	// of whatever accumulated errors we got here.
	select {
	case <-bf.ic.Done():
		return bf.ic.Err()
	default:
		return err
	}
}

// RunBatch is a batching version of Run. Like Run, executes a query and invokes
// the supplied callback for each returned result. RunBatch differs from Run in
// that it performs the query in batches, using a cursor to continue the query
// in between batches.
//
// See Run for more information about the parameters.
//
// Batching processes the supplied query in batches, buffering the full batch
// set locally before sending its results to the user. It will then proceed to
// the next batch until finished or cancelled. This is useful:
//	- For efficiency, decoupling the processing of query data from the
//	  underlying datastore operation.
//	- For very long-running queries, where the duration of the query would
//	  normally exceed datastore's maximum query timeout.
//	- The caller may count return callbacks and perform processing at each
//	  `batchSize` interval with confidence that the underlying query will not
//	  timeout during that processing.
//
// If the Context supplied to RunBatch is cancelled or reaches its deadline,
// RunBatch will terminate with the Context's error.
//
// By default, datastore applies a short (~5s) timeout to queries. This can be
// increased, usually to around several minutes, by explicitly setting a
// deadline on the supplied Context.
//
// If the specified `batchSize` is <= 0, no batching will be performed.
func RunBatch(c context.Context, batchSize int32, q *Query, cb interface{}) error {
	return Run(withQueryBatching(c, batchSize), q, cb)
}

// CountBatch is a batching version of Count. See RunBatch for more information
// about batching, and CountBatch for more information about the parameters.
//
// If the Context supplied to CountBatch is cancelled or reaches its deadline,
// CountBatch will terminate with the Context's error.
//
// By default, datastore applies a short (~5s) timeout to queries. This can be
// increased, usually to around several minutes, by explicitly setting a
// deadline on the supplied Context.
//
// If the specified `batchSize` is <= 0, no batching will be performed.
func CountBatch(c context.Context, batchSize int32, q *Query) (int64, error) {
	return Count(withQueryBatching(c, batchSize), q)
}

func withQueryBatching(c context.Context, batchSize int32) context.Context {
	if batchSize <= 0 {
		return c
	}

	return AddRawFilters(c, func(ic context.Context, raw RawInterface) RawInterface {
		return &queryBatchingFilter{
			RawInterface: raw,
			ic:           ic,
			batchSize:    batchSize,
		}
	})
}

type queryBatchingFilter struct {
	RawInterface

	ic        context.Context
	batchSize int32
}

func (f *queryBatchingFilter) Run(fq *FinalizedQuery, cb RawRunCB) error {
	limit, hasLimit := fq.Limit()

	// Buffer for each batch.
	type batchEntry struct {
		key       *Key
		val       PropertyMap
		getCursor CursorCB
	}
	buffer := make([]batchEntry, 0, int(f.batchSize))

	// Install an intermediate callback so we can iteratively batch.
	var nextCursor Cursor
	for {
		// Has our Context been cancelled?
		select {
		case <-f.ic.Done():
			return f.ic.Err()
		default:
		}

		iterQuery := fq.Original()
		if nextCursor != nil {
			iterQuery = iterQuery.Start(nextCursor)
			nextCursor = nil
		}
		iterLimit := f.batchSize
		if hasLimit && limit < iterLimit {
			iterLimit = limit
		}
		iterQuery = iterQuery.Limit(iterLimit)

		iterFinalizedQuery, err := iterQuery.Finalize()
		if err != nil {
			panic(fmt.Errorf("failed to finalize internal query: %v", err))
		}

		err = f.RawInterface.Run(iterFinalizedQuery, func(key *Key, val PropertyMap, getCursor CursorCB) error {
			if nextCursor != nil {
				// We're iterating past our batch size, which should never happen, since
				// we set a limit. This will only happen when our inner RawInterface
				// fails to honor the limit that we set.
				panic(fmt.Errorf("iterating past batch size"))
			}

			// If this entry would complete the batch,  get the cursor for the next
			// batch.
			if len(buffer)+1 >= int(f.batchSize) {
				cursor, err := getCursor()
				if err != nil {
					return fmt.Errorf("failed to get cursor: %v", err)
				}

				// If the caller wants to get the cursor, we can avoid an extra RPC
				// by just supplying it.
				getCursor = func() (Cursor, error) { return cursor, nil }
				nextCursor = cursor
			}

			buffer = append(buffer, batchEntry{
				key:       key,
				val:       val,
				getCursor: getCursor,
			})
			return nil
		})
		if err != nil {
			return err
		}

		// Invoke our callback for each buffered entry.
		for i := range buffer {
			ent := &buffer[i]
			if err := cb(ent.key, ent.val, ent.getCursor); err != nil {
				return err
			}
		}

		// If we have no next cursor, we're done.
		if nextCursor == nil {
			break
		}

		// Reduce our limit for the next round.
		if hasLimit {
			limit -= int32(len(buffer))
			if limit <= 0 {
				break
			}
		}

		// Reset our buffer for the next round.
		buffer = buffer[:0]
	}
	return nil
}
