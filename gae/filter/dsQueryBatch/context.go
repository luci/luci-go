// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dsQueryBatch

import (
	ds "github.com/luci/gae/service/datastore"
	"golang.org/x/net/context"
)

// BatchQueries installs a datastore filter that causes all queries to be broken
// into a series of iterative fixed-size queries. The batching uses cursors to
// chain the iterations together.
//
// This helps accommodate query size or time limits enforced by the backing
// datastore implementation.
//
// Note that this expands a single query into a series of queries, which may
// lose additional single-query consistency guarantees.
func BatchQueries(c context.Context, batchSize int32) context.Context {
	return ds.AddRawFilters(c, func(ic context.Context, ri ds.RawInterface) ds.RawInterface {
		return &iterQueryFilter{
			RawInterface: ri,
			batchSize:    batchSize,
		}
	})
}
