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

package buildbot

import (
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/gae/service/datastore"

	"golang.org/x/net/context"
)

// buildQueryBatchSize is the batch size to use when querying build.
//
// This should be tuned to the observed query timeout for build loading. Since
// loading is CPU-bound, this will probably be low. If build queries start
// encountering datastore timeouts, reduce this value.
const buildQueryBatchSize = int32(50)

// masterQueryBatchSize is the batch size to use when querying masters.
const masterQueryBatchSize = int32(500)

// runBuildsQuery takes a buildbotBuild query and returns a list of builds
// along with a cursor. We pass the limit here and apply it to the query as
// an optimization because then we can create a build container of that size.
func runBuildsQuery(c context.Context, q *datastore.Query) ([]*buildbotBuild, datastore.Cursor, error) {

	// Finalize the input query so we can pull its limit.
	fq, err := q.Finalize()
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not finalize query").Err()
	}

	limit, hasLimit := fq.Limit()

	buildsAllocSize := buildQueryBatchSize
	if hasLimit {
		buildsAllocSize = limit
	}
	builds := make([]*buildbotBuild, 0, buildsAllocSize)

	var nextCursor datastore.Cursor
	err = datastore.RunBatch(c, buildQueryBatchSize, q, func(build *buildbotBuild, getCursor datastore.CursorCB) error {
		builds = append(builds, build)

		// Only capture the cursor if we actually hit our limit. Fetching the
		// cursor is an expensive operation.
		if hasLimit && len(builds) >= int(limit) {
			var err error
			if nextCursor, err = getCursor(); err != nil {
				return err
			}
		}
		return nil
	})
	return builds, nextCursor, err
}

func queryAllMasters(c context.Context) ([]*buildbotMasterEntry, error) {
	q := datastore.NewQuery(buildbotMasterEntryKind)
	entries := make([]*buildbotMasterEntry, 0, masterQueryBatchSize)
	err := datastore.RunBatch(c, masterQueryBatchSize, q, func(e *buildbotMasterEntry) {
		entries = append(entries, e)
	})
	return entries, err
}
