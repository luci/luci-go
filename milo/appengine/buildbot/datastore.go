// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	ds "github.com/luci/gae/service/datastore"

	"golang.org/x/net/context"
)

// buildQueryBatchSize is the batch size to use when querying build. It is
// employed as an upper bound by getBuildQueryBatcher.
//
// This should be tuned to the observed query timeout for build loading. Since
// loading is CPU-bound, this will probably be low. If build queries start
// encountering datastore timeouts, reduce this value.
const buildQueryBatchSize = 50

// getBuildQueryBatcher returns a ds.Batcher tuned for executing queries on the
// "buildbotBuild" entity.
func getBuildQueryBatcher(c context.Context) *ds.Batcher {
	constraints := ds.Raw(c).Constraints()
	if constraints.QueryBatchSize > buildQueryBatchSize {
		constraints.QueryBatchSize = buildQueryBatchSize
	}
	return &ds.Batcher{
		Size: constraints.QueryBatchSize,
	}
}
