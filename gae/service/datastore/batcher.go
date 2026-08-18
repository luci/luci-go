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
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
)

func applyBatchFilter(c context.Context, rds RawInterface) RawInterface {
	if !getBatchingEnabled(c) {
		return rds
	}

	return &batchFilter{
		RawInterface: rds,
		ic:           c,
		constraints:  rds.Constraints(),
	}
}

type batchFilter struct {
	RawInterface

	ic          context.Context
	constraints Constraints
}

func (bf *batchFilter) GetMulti(keys []*Key, meta MultiMetaGetter, cb GetMultiCB) error {
	return bf.batchParallel(len(keys), bf.constraints.MaxGetSize, func(offset, count int) error {
		return bf.RawInterface.GetMulti(keys[offset:offset+count], meta, func(idx int, val PropertyMap, err error) {
			cb(offset+idx, val, err)
		})
	})
}

func (bf *batchFilter) PutMulti(keys []*Key, vals []PropertyMap, cb NewKeyCB) error {
	return bf.batchParallel(len(vals), bf.constraints.MaxPutSize, func(offset, count int) error {
		return bf.RawInterface.PutMulti(keys[offset:offset+count], vals[offset:offset+count], func(idx int, key *Key, err error) {
			cb(offset+idx, key, err)
		})
	})
}

func (bf *batchFilter) DeleteMulti(keys []*Key, cb DeleteMultiCB) error {
	return bf.batchParallel(len(keys), bf.constraints.MaxDeleteSize, func(offset, count int) error {
		return bf.RawInterface.DeleteMulti(keys[offset:offset+count], func(idx int, err error) {
			cb(offset+idx, err)
		})
	})
}

func (bf *batchFilter) batchParallel(count, batch int, cb func(offset, count int) error) error {
	// If no batch size is defined, or if this can be done in one batch, then do
	// everything in a single batch.
	if batch <= 0 || count <= batch {
		return cb(0, count)
	}

	batching := batch > 0

	// If batching is disabled, we will skip goroutines and do everything in a
	// single batch.
	if !batching {
		if batch > 0 && count > batch {
			return errors.Fmt("batching is disabled, and size (%d) exceeds maximum (%d)", count, batch)
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

// RunBatchQuery is a batching version of RunQuery. Like RunQuery, executes a query
// and returns a QueryIter[V]. RunBatchQuery differs from RunQuery in that it performs
// the query in batches, using a cursor to continue the query in between batches.
//
// NOTE: QueryIter[V].Cursor is NOT supported when using batched processing mode.
//
// See RunQuery for more information about the parameters.
//
// Batching processes the supplied query in batches, buffering the full batch
// set locally before sending its results to the user. It will then proceed to
// the next batch until finished or cancelled. This is useful:
//   - For efficiency, decoupling the processing of query data from the
//     underlying datastore operation.
//   - For very long-running queries, where the duration of the query would
//     normally exceed datastore's maximum query timeout.
//   - The caller may count returned items and perform processing at each
//     `batchSize` interval with confidence that the underlying query will not
//     timeout during that processing.
//
// If the Context supplied to RunBatchQuery is cancelled or reaches its deadline,
// RunBatchQuery will terminate with the Context's error.
//
// By default, datastore applies a short (~5s) timeout to queries. This can be
// increased, usually to around several minutes, by explicitly setting a
// deadline on the supplied Context.
//
// If the specified `batchSize` is <= 0, no batching will be performed.
func RunBatchQuery[V any](c context.Context, batchSize int32, q *Query) *QueryIter[V] {
	return RunQuery[V](withQueryBatching(c, batchSize), q)
}

// CountBatch is a batching version of Count. See [RunBatchQuery] for more
// information about batching, and [Count] for more information about the
// parameters.
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

func (f *queryBatchingFilter) RunQuery(fq *FinalizedQuery) RawQueryIter {
	limit, hasLimit := fq.Limit()

	return RawQueryIter{
		Cursor: func() (RawCursor, error) {
			return nil, errors.Fmt("queryBatchingFilter: %w", ErrCursorNotImplemented)
		},
		Results: func(yield func(PropertyMap, error) bool) {
			var buffer []PropertyMap
			var nextCursor RawCursor

			for {
				select {
				case <-f.ic.Done():
					yield(nil, f.ic.Err())
					return
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

				it := f.RawInterface.RunQuery(iterFinalizedQuery)
				for pm, err := range it.Results {
					if err != nil {
						yield(nil, err)
						return
					}
					buffer = append(buffer, pm)
					if len(buffer) >= int(f.batchSize) {
						cursor, err := it.Cursor()
						if err != nil {
							yield(nil, fmt.Errorf("failed to get cursor: %v", err))
							return
						}
						nextCursor = cursor
						break
					}
				}

				if len(buffer) == 0 {
					return
				}

				for _, pm := range buffer {
					if hasLimit {
						if limit <= 0 {
							return
						}
						limit--
					}
					if !yield(pm, nil) {
						return
					}
				}

				if hasLimit && limit <= 0 {
					return
				}

				if nextCursor == nil || int64(len(buffer)) < int64(f.batchSize) {
					return
				}
				buffer = buffer[:0]
			}
		},
	}
}
