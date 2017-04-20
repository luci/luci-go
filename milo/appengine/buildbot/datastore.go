// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"github.com/luci/gae/service/datastore"

	"golang.org/x/net/context"
)

// buildQueryBatchSize is the batch size to use when querying build. It is
// employed as an upper bound by getBuildQueryBatcher.
//
// This should be tuned to the observed query timeout for build loading. Since
// loading is CPU-bound, this will probably be low. If build queries start
// encountering datastore timeouts, reduce this value.
const buildQueryBatchSize = 50

// getBuildQueryBatcher returns a datastore.Batcher tuned for executing queries on the
// "buildbotBuild" entity.
func getBuildQueryBatcher(c context.Context) *datastore.Batcher {
	constraints := datastore.Raw(c).Constraints()
	if constraints.QueryBatchSize > buildQueryBatchSize {
		constraints.QueryBatchSize = buildQueryBatchSize
	}
	return &datastore.Batcher{
		Size: constraints.QueryBatchSize,
	}
}

// runBuildsQuery takes a buildbotBuild query and returns a list of builds
// along with a cursor.  We pass the limit here and apply it to the query as
// an optimization because then we can create a build container of that size.
func runBuildsQuery(c context.Context, q *datastore.Query, limit int32) (
	[]*buildbotBuild, *datastore.Cursor, error) {

	if limit != 0 {
		q = q.Limit(limit)
	}
	builds := make([]*buildbotBuild, 0, limit)
	var nextCursor *datastore.Cursor
	err := getBuildQueryBatcher(c).Run(
		c, q, func(build *buildbotBuild, getCursor datastore.CursorCB) error {
			builds = append(builds, build)
			tmpCursor, err := getCursor()
			if err != nil {
				return err
			}
			nextCursor = &tmpCursor
			return nil
		})
	return builds, nextCursor, err
}
