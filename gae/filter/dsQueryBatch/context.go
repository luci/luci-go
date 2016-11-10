// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dsQueryBatch

import (
	ds "github.com/luci/gae/service/datastore"
	"golang.org/x/net/context"
)

// Callback is a callback that is executed after query batches.
type Callback func(context.Context) error

// BatchQueries installs a datastore filter that causes all queries to be broken
// into a series of iterative fixed-size queries. The batching uses cursors to
// chain the iterations together.
//
// This helps accommodate query size or time limits enforced by the backing
// datastore implementation.
//
// Note that this expands a single query into a series of queries, which may
// lose additional single-query consistency guarantees.
//
// If callbacks are supplied, they will be invoked in order in between each
// batch of queries. These invocations guarantee that the callback will not
// interrupt any ongoing queries, allowing potentially time-consuming operations
// to be performed. If a callbacks returns an error, no more callbacks will be
// invoked and the query will immediately stop and return that error.
func BatchQueries(c context.Context, batchSize int32, cb ...Callback) context.Context {
	return ds.AddRawFilters(c, func(ic context.Context, ri ds.RawInterface) ds.RawInterface {
		return &iterQueryFilter{
			RawInterface: ri,

			ctx:       ic,
			batchSize: batchSize,
			callbacks: cb,
		}
	})
}
