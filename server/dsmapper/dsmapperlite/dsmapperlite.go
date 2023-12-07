// Copyright 2023 The LUCI Authors.
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

// Package dsmapperlite implements an in-process datastore mapper.
//
// Unlike its bigger sibling dsmapper, it doesn't distribute mapping
// operations across machines, but in exchange has a very simple API. There's
// no need to install it as a server module or setup task queue tasks etc. Just
// use is a library.
//
// Useful for quickly visiting up to 100K entities.
package dsmapperlite

import (
	"context"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/dsmapper/internal/splitter"
)

// Map passes all entities matching the query to the callback, in parallel,
// in some random order.
//
// Runs up to `shards` number of parallel goroutines, where each one executes
// a datastore query and passes the resulting entities to the callback (along
// with the shard index). Each query fetches entities in `batchSize` pages
// before handling them. The overall memory consumption is thus
// `O(shards * batchSize * averageEntitySize)`.
//
// Within a shard, the callback is called sequentially, but different shards
// are processed in parallel. If the callback needs to parallelize entity
// processing more, it should manage its own goroutine pool and pass entities
// to it.
//
// If the callback returns an error, Map aborts the entire operation ASAP (but
// it may take some time to wind down). When this happens, the context passed
// to the callback is canceled.
func Map[E any](ctx context.Context, q *datastore.Query, shards, batchSize int, cb func(ctx context.Context, shard int, entity E) error) error {
	logging.Infof(ctx, "Calculating ranges...")
	ranges, err := splitter.SplitIntoRanges(ctx, q, splitter.Params{
		Shards:  shards,
		Samples: 500,
	})
	if err != nil {
		return errors.Annotate(err, "failed to do the initial __scatter__ query").Err()
	}
	logging.Infof(ctx, "Querying %d ranges in parallel...", len(ranges))
	eg, ctx := errgroup.WithContext(ctx)
	for idx, r := range ranges {
		idx := idx
		rangedQ := r.Apply(q)
		eg.Go(func() error {
			return datastore.RunBatch(ctx, int32(batchSize), rangedQ, func(e E) error {
				return cb(ctx, idx, e)
			})
		})
	}
	return eg.Wait()
}
