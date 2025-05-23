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
	"container/heap"
	"context"
	"fmt"

	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/run"
)

// RecentQueryBuilder builds a VERY SLOW query for searching recent Runs
// across all LUCI projects.
//
// If two runs have the same timestamp, orders Runs first by the LUCI Project
// name and then by the remainder of the Run's ID.
//
// Beware: two Runs having the same timestamp is actually quite likely with
// Google Gerrit because it rounds updates to second granularity, which then
// makes its way as Run Creation time.
//
// **WARNING**: this is the most inefficient way to be used infrequently for CV
// admin needs only. Behind the scenes, it issues a Datastore query per active
// LUCI project.
//
// Doesn't yet support restricting search to a specific time range, but it can
// be easily implemented if necessary.
type RecentQueryBuilder struct {
	// Status optionally restricts query to Runs with this status.
	Status run.Status
	// Limit limits the number of results if positive. Ignored otherwise.
	Limit int32
	// CheckProjectAccess checks if the calling user has access to the LUCI
	// project. Optional.
	//
	// If provided, Runs from the LUCI Projects that requested user doesn't have
	// access to won't be returned without even querying.
	// If not provided, search is done across all projects.
	CheckProjectAccess func(context.Context, string) (bool, error)

	// lastFoundRunID is basically a page token.
	lastFoundRunID common.RunID
	// boundaryInverseTS / boundaryProject are just caches based on
	// lastFoundRunID set by PageToken(). See keysForProject() for their use.
	boundaryInverseTS string
	boundaryProject   string

	// availableProjects is set on a local temp copy of the RecentQueryBuilder by
	// the LoadRuns(). It's then used by loadAvailableProjects as a cache if not
	// nil. Empty slice means a cached value of no available projects,
	// possibly because caller has no access to any project.
	availableProjects []string
}

// PageToken constraints RecentQueryBuilder to continue searching from the
// prior search.
func (b RecentQueryBuilder) PageToken(pt *PageToken) RecentQueryBuilder {
	if pt != nil {
		b.lastFoundRunID = common.RunID(pt.GetRun())
		b.boundaryProject = b.lastFoundRunID.LUCIProject()
		b.boundaryInverseTS = b.lastFoundRunID.InverseTS()
	}
	return b
}

// GetAllRunKeys runs the query and returns Datastore keys to Run entities.
//
// WARNING: very slow.
//
// Since RunID includes LUCI project, RunIDs aren't lexicographically ordered by
// creation time across LUCI projects. So, the brute force is to query each
// known to CV LUCI project for most recent Run IDs, and then merge and select
// the next page of resulting keys.
func (b RecentQueryBuilder) GetAllRunKeys(ctx context.Context) ([]*datastore.Key, error) {
	projects, err := b.loadAvailableProjects(ctx)
	switch {
	case err != nil:
		return nil, err
	case len(projects) == 0:
		return nil, nil
	}

	// Do a query per project, in parallel.
	// KeysOnly queries are cheap in both time and money.
	allKeys := make([][]*datastore.Key, len(projects))
	errs := parallel.WorkPool(min(16, len(projects)), func(work chan<- func() error) {
		for i, p := range projects {
			work <- func() error {
				var err error
				allKeys[i], err = b.keysForProject(ctx, p)
				return err
			}
		}
	})
	if errs != nil {
		return nil, common.MostSevereError(errs)
	}
	// Finally, merge resulting keys maintaining the documented order.
	return b.selectLatest(allKeys...), nil
}

// LoadRuns returns matched Runs and the page token to continue search later.
func (b RecentQueryBuilder) LoadRuns(ctx context.Context, checkers ...run.LoadRunChecker) ([]*run.Run, *PageToken, error) {
	// Load all enabled & disabled projects and set the cache.
	//
	// NOTE: the cache is bound to this copy of RecentQueryBuilder and whichever
	// copies are made from it by the loadRunsFromQuery, but the caller of the
	// RecentQueryBuilder.LoadRuns never gets to see such a copy.
	switch projects, err := b.loadAvailableProjects(ctx); {
	case err != nil:
		return nil, nil, err
	case len(projects) == 0:
		b.availableProjects = []string{}
	default:
		b.availableProjects = projects
	}

	return loadRunsFromQuery(ctx, b, checkers...)
}

// loadAvailableProjects caches active visible projects.
func (b RecentQueryBuilder) loadAvailableProjects(ctx context.Context) ([]string, error) {
	// Use cache if it is set.
	// NOTE: Empty slice is a valid cache value of 0 available projects.
	if b.availableProjects != nil {
		return b.availableProjects, nil
	}

	// Load all enabled & disabled projects.
	projects, err := prjcfg.GetAllProjectIDs(ctx, false)
	if err != nil {
		return nil, err
	}
	if b.CheckProjectAccess == nil {
		return projects, nil
	}
	filtered := projects[:0]
	for _, p := range projects {
		switch ok, err := b.CheckProjectAccess(ctx, p); {
		case err != nil:
			return nil, err
		case ok:
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

// qLimit implements runKeysQuery interface.
func (b RecentQueryBuilder) qLimit() int32 { return b.Limit }

// qPageToken implements runKeysQuery interface.
func (b RecentQueryBuilder) qPageToken(pt *PageToken) runKeysQuery { return b.PageToken(pt) }

// isSatisfied returns whether the given Run satisfies the query.
func (b RecentQueryBuilder) isSatisfied(r *run.Run) bool {
	switch {
	case r == nil:
	case b.Status == run.Status_ENDED_MASK && !run.IsEnded(r.Status):
	case b.Status != run.Status_ENDED_MASK && b.Status != run.Status_STATUS_UNSPECIFIED && r.Status != b.Status:
	default:
		return true
	}
	return false
}

// keysForProject returns matching Run keys for a specific project.
func (b RecentQueryBuilder) keysForProject(ctx context.Context, project string) ([]*datastore.Key, error) {
	pqb := ProjectQueryBuilder{
		Status:  b.Status,
		Project: project,
		Limit:   b.Limit,
	}
	switch {
	case b.boundaryProject == "":
		// No page token.
	case b.boundaryProject > project:
		// Must be a strictly older Run, i.e. have strictly higher InverseTS than
		// the boundaryInverseTS. Since '-' (ASCII code 45) follows the InverseTS
		// in RunID schema, all Run IDs with the same InverseTS will be smaller
		// than 'InverseTS.' ('.' has ASCII code 46).
		pqb.MinExcl = common.RunID(fmt.Sprintf("%s/%s%c", project, b.boundaryInverseTS, ('-' + 1)))
	case b.boundaryProject < project:
		// Must have the same or higher InverseTS.
		// Since '-' follows the InverseTS in RunID schema, any RunID with the
		// same InverseTS will be strictly greater than "InverseTS-".
		pqb.MinExcl = common.RunID(fmt.Sprintf("%s/%s%c", project, b.boundaryInverseTS, '-'))
	default:
		// Same LUCI project, can use the last found Run as the boundary.
		// This is actually important in case there are several Runs in this project
		// with the same timestamp.
		pqb.MinExcl = b.lastFoundRunID
	}
	return pqb.GetAllRunKeys(ctx)
}

// selectLatest returns up to the limit of Run IDs ordered by:
//   - DESC Created (== ASC InverseTS, or latest first)
//   - ASC  Project
//   - ASC  RunID (the remaining part of RunID)
//
// IDs in each input slice must be be in Created DESC order.
//
// Mutates inputs.
func (b RecentQueryBuilder) selectLatest(inputs ...[]*datastore.Key) []*datastore.Key {
	popLatest := func(idx int) (runHeapKey, bool) {
		input := inputs[idx]
		if len(input) == 0 {
			return runHeapKey{}, false
		}
		inputs[idx] = input[1:]
		rid := common.RunID(input[0].StringID())
		inverseTS := rid.InverseTS()
		project := rid.LUCIProject()
		remaining := rid[len(project)+1+len(inverseTS):]
		sortKey := fmt.Sprintf("%s/%s/%s", inverseTS, project, remaining)
		return runHeapKey{input[0], sortKey, idx}, true
	}

	h := make(runHeap, 0, len(inputs))
	// Init the heap with the latest element from each non-empty input.
	for idx := range inputs {
		if v, ok := popLatest(idx); ok {
			h = append(h, v)
		}
	}
	heap.Init(&h)

	var out []*datastore.Key
	for len(h) > 0 {
		v := heap.Pop(&h).(runHeapKey)
		out = append(out, v.dsKey)
		if len(out) == int(b.Limit) { // if Limit is <= 0, continues until heap is empty.
			break
		}
		if v, ok := popLatest(v.idx); ok {
			heap.Push(&h, v)
		}
	}
	return out
}

// runHeapKey facilitates heap-based merge of multiple consistently sorted
// ranges of Datastore keys, each range identified by its index.
type runHeapKey struct {
	dsKey   *datastore.Key
	sortKey string
	idx     int
}
type runHeap []runHeapKey

func (r runHeap) Len() int {
	return len(r)
}

func (r runHeap) Less(i int, j int) bool {
	return r[i].sortKey < r[j].sortKey
}

func (r runHeap) Swap(i int, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r *runHeap) Push(x any) {
	*r = append(*r, x.(runHeapKey))
}

func (r *runHeap) Pop() any {
	idx := len(*r) - 1
	v := (*r)[idx]
	(*r)[idx].dsKey = nil // free memory as a good habit.
	*r = (*r)[:idx]
	return v
}
