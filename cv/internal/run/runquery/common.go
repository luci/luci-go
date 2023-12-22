// Copyright 2020 The LUCI Authors.
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

package runquery

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/run"
)

// rangeOfProjectIDs returns (min..max) non-existent Run IDs, such that
// the following
//
//	min < $RunID  < max
//
// for all valid $RunID belonging to the given project.
func rangeOfProjectIDs(project string) (string, string) {
	// ID starts with the LUCI Project name followed by '/' and a 13-digit
	// number. So it must be lexicographically greater than "project/0" and
	// less than "project/:" (int(':') == int('9') + 1).
	return project + "/0", project + "/:"
}

// runKeysQuery abstracts out existing ...QueryBuilder in this file.
type runKeysQuery interface {
	GetAllRunKeys(context.Context) ([]*datastore.Key, error)
	isSatisfied(*run.Run) bool
	qPageToken(*PageToken) runKeysQuery // must return a copy
	qLimit() int32
}

// queryStopAfterDuration and queryStopAfterIterations gracefully stop
// loadRunsFromQuery after both are reached.
const queryStopAfterDuration = 5 * time.Second
const queryStopAfterIterations = 5

// loadRunsFromQuery returns matched Runs and a page token.
//
// If limit is specified, continues loading Runs until the limit is reached.
func loadRunsFromQuery(ctx context.Context, q runKeysQuery, checkers ...run.LoadRunChecker) ([]*run.Run, *PageToken, error) {
	if l := len(checkers); l > 1 {
		panic(fmt.Errorf("at most 1 LoadRunChecker allowed, %d given", l))
	}

	var out []*run.Run

	// Loop until we fetch the limit.
	startTime := clock.Now(ctx)
	originalQuery := q
	limit := int(q.qLimit())
	for iteration := 1; ; iteration++ {
		// Fetch the next `limit` of keys in all iterations, even though we may
		// already have some in `out` since the keys-only queries are cheap,
		// and this way code is simpler.
		keys, err := q.GetAllRunKeys(ctx)
		switch {
		case err != nil:
			return nil, nil, err
		case len(keys) == 0:
			// Search space exhausted.
			return out, nil, nil
		}

		loader := run.LoadRunsFromKeys(keys...)
		if len(checkers) == 1 {
			loader = loader.Checker(checkers[0])
		}
		runs, err := loader.DoIgnoreNotFound(ctx)
		if err != nil {
			return nil, nil, err
		}
		// Even for queries which can do everything using native Datastore
		// query, there is a window of time between the Datastore query
		// fetching keys of satisfying Runs and actual Runs being fetched.
		// During this window, the Runs ultimately fetched could have been
		// modified, so check again and skip all fetched Runs which no longer
		// satisfy the query.
		for _, r := range runs {
			if q.isSatisfied(r) {
				out = append(out, r)
			}
		}

		switch {
		case limit <= 0:
			return out, nil, nil

		case len(out) < limit:
			// Prepare to iterating from the last considered key.
			pt := &PageToken{Run: keys[len(keys)-1].StringID()}
			q = q.qPageToken(pt)
			if iteration >= queryStopAfterIterations && clock.Since(ctx, startTime) > queryStopAfterDuration {
				// Avoid excessive looping when most Runs are filtered out by
				// returning an incomplete page of results earlier with a valid
				// page token, so that if necessary, the caller can continue
				// later.
				logging.Debugf(ctx, "loadRunsFromQuery stops after %d iterations and %s time, returning %d out of %d requested Runs [query: %s]",
					iteration, clock.Since(ctx, startTime), len(out), limit, originalQuery)
				return out, pt, nil
			}

		case len(out) == limit && len(keys) < limit:
			// Even though some Runs may have been filtered by checkers and/or
			// isSatisfied called, we know we processed all the keys most recently
			// fetched and there are no more keys.
			// So, page token is not necessary.
			return out, nil, nil

		case len(out) == limit:
			return out, &PageToken{Run: keys[len(keys)-1].StringID()}, nil

		default:
			firstNotReturnedID := string(out[limit].ID)
			// The firstNotReturnedID would have been a perfect page token iff
			// *inclusive* PageToken was supported. But since we need an *exclusive*
			// page token, find a key preceding the key corresponding to
			// firstNotReturnedID.
			//
			// NOTE: the keys themselves aren't necessarily ordered by ASC Run ID,
			// e.g. in case of RecentQueryBuilder. Therefore, we must iterate them in
			// order and can't do binary search.
			for i, k := range keys {
				if k.StringID() == firstNotReturnedID {
					return out[:limit], &PageToken{Run: keys[i-1].StringID()}, nil
				}
			}
			panic("unreachable")
		}
	}
}
