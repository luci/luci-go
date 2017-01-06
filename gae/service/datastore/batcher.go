// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"fmt"
	"math"

	"github.com/luci/luci-go/common/errors"

	"golang.org/x/net/context"
)

// Batcher is an augmentation to the top-level datastore API that processes
// functions in batches. This can be used to avoid per-operation timeouts that
// the top-level API is subject to.
//
// A note on queries:
// BatchQueries installs a datastore filter that causes all queries to be broken
// into a series of iterative fixed-size queries. The Batcher uses cursors to
// chain the iterations together.
//
// This helps accommodate query size or time limits enforced by the backing
// datastore implementation.
//
// Note that this expands a single query into a series of queries, which may
// lose additional single-query consistency guarantees.
type Batcher struct {
	// Callback, if not nil, is called in between batch iterations. If the
	// callback returns an error, the error will be returned by the top-level
	// operation, and no further batches will be executed.
	//
	// When querying, the Callback will be executed in between query operations,
	// meaning that the time consumed by the callback will not run the risk of
	// timing out any individual query.
	Callback func(context.Context) error

	// Size is the batch size. If it's <= 0, a default batch size will be chosen
	// based on the batching function and the implementation's constraints.
	Size int
}

// Run executes the given query, and calls `cb` for each successfully
// retrieved item. See the top-level Run for more semantics.
//
// If the specified batch size is <= 0, the current implementation's
// QueryBatchSize constraint will be used.
func (b *Batcher) Run(c context.Context, q *Query, cb interface{}) error {
	raw := rawWithFilters(c, applyBatchQueryFilter(b))
	return runRaw(raw, q, cb)
}

// GetAll returns all results for the given query, and calls `cb` for each
// successfully retrieved item. See the top-level GetAll for more semantics.
//
// If the specified batch size is <= 0, the current implementation's
// QueryBatchSize constraint will be used.
func (b *Batcher) GetAll(c context.Context, q *Query, dst interface{}) error {
	raw := rawWithFilters(c, applyBatchQueryFilter(b))
	return getAllRaw(raw, q, dst)
}

func (b *Batcher) runCallback(c context.Context) error {
	if b.Callback == nil {
		return nil
	}
	return b.Callback(c)
}

type batchQueryFilter struct {
	RawInterface

	b  *Batcher
	ic context.Context
}

func applyBatchQueryFilter(b *Batcher) RawFilter {
	return func(ic context.Context, rds RawInterface) RawInterface {
		return &batchQueryFilter{
			RawInterface: rds,
			b:            b,
			ic:           ic,
		}
	}
}

func (bqf *batchQueryFilter) Run(fq *FinalizedQuery, cb RawRunCB) error {
	// Determine batch size.
	batchSize := bqf.b.Size
	if batchSize <= 0 {
		batchSize = bqf.Constraints().QueryBatchSize
	}

	switch {
	case batchSize <= 0:
		return bqf.RawInterface.Run(fq, cb)
	case batchSize > math.MaxInt32:
		return errors.New("batch size must fit in int32")
	}
	bs := int32(batchSize)
	limit, hasLimit := fq.Limit()

	// Install an intermediate callback so we can iteratively batch.
	var cursor Cursor
	for {
		iterQuery := fq.Original()
		if cursor != nil {
			iterQuery = iterQuery.Start(cursor)
			cursor = nil
		}
		iterLimit := bs
		if hasLimit && limit < iterLimit {
			iterLimit = limit
		}
		iterQuery = iterQuery.Limit(iterLimit)

		iterFinalizedQuery, err := iterQuery.Finalize()
		if err != nil {
			panic(fmt.Errorf("failed to finalize internal query: %v", err))
		}

		count := int32(0)
		err = bqf.RawInterface.Run(iterFinalizedQuery, func(key *Key, val PropertyMap, getCursor CursorCB) error {
			if cursor != nil {
				// We're iterating past our batch size, which should never happen, since
				// we set a limit. This will only happen when our inner RawInterface
				// fails to honor the limit that we set.
				panic(fmt.Errorf("iterating past batch size"))
			}

			if err := cb(key, val, getCursor); err != nil {
				return err
			}

			// If this is the last entry in our batch, get the cursor and stop this
			// query round.
			count++
			if count >= bs {
				if cursor, err = getCursor(); err != nil {
					return fmt.Errorf("failed to get cursor: %v", err)
				}
				return Stop
			}
			return nil
		})
		if err != nil {
			return err
		}

		// If we have no cursor, we're done.
		if cursor == nil {
			break
		}

		// Reduce our limit for the next round.
		if hasLimit {
			limit -= count
			if limit <= 0 {
				break
			}
		}

		// Execute our callback(s).
		if err := bqf.b.runCallback(bqf.ic); err != nil {
			return err
		}
	}
	return nil
}
