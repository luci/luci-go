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

package run

import (
	"container/heap"
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
)

// CLQueryBuilder builds datastore.Query for searching Runs of a given CL.
type CLQueryBuilder struct {
	// CLID of the CL being searched for. Required.
	CLID common.CLID
	// Project optionally restricts Runs to the given LUCI project.
	Project string
	// MaxExcl restricts query to Runs with ID lexicographically smaller.
	//
	// This means query will return union of:
	//   * all Runs created after this Run in the same project,
	//   * all Runs in lexicographically smaller projects,
	//     unless .Project is set to the same project (recommended).
	MaxExcl common.RunID
	// MinExcl restricts query to Runs with ID lexicographically larger.
	//
	// This means query will return union of:
	//   * all Runs created before this Run in the same project,
	//   * all Runs in lexicographically larger projects,
	//     unless .Project is set to the same project (recommended).
	MinExcl common.RunID

	// Limit limits the number of results if positive. Ignored otherwise.
	Limit int32
}

// isSatisfied returns whether the given Run satisfies the query.
func (b CLQueryBuilder) isSatisfied(r *Run) bool {
	switch {
	case r == nil:
	case b.Project != "" && r.ID.LUCIProject() != b.Project:
	case b.MinExcl != "" && r.ID <= b.MinExcl:
	case b.MaxExcl != "" && r.ID >= b.MaxExcl:
	default:
		return true
	}
	return false
}

// AfterInProject constrains CLQueryBuilder to Runs created after this Run but
// belonging to the same LUCI project.
//
// Panics if CLQueryBuilder is already constrained to a different LUCI Project.
func (b CLQueryBuilder) AfterInProject(id common.RunID) CLQueryBuilder {
	if p := id.LUCIProject(); p != b.Project {
		if b.Project != "" {
			panic(fmt.Errorf("invalid CLQueryBuilder.AfterInProject(%q): .Project is already set to %q", id, b.Project))
		}
		b.Project = p
	}
	b.MaxExcl = id
	return b
}

// BeforeInProject constrains CLQueryBuilder to Runs created before this Run but
// belonging to the same LUCI project.
//
// Panics if CLQueryBuilder is already constrained to a different LUCI Project.
func (b CLQueryBuilder) BeforeInProject(id common.RunID) CLQueryBuilder {
	if p := id.LUCIProject(); p != b.Project {
		if b.Project != "" {
			panic(fmt.Errorf("invalid CLQueryBuilder.BeforeInProject(%q): .Project is already set to %q", id, b.Project))
		}
		b.Project = p
	}
	b.MinExcl = id
	return b
}

// PageToken constraints CLQueryBuilder to continue searching from the prior
// search.
func (b CLQueryBuilder) PageToken(pt *PageToken) CLQueryBuilder {
	if pt != nil {
		b.MinExcl = common.RunID(pt.GetRun())
	}
	return b
}

// BuildKeysOnly returns keys-only query on RunCL entities.
//
// It's exposed primarily for debugging reasons.
func (b CLQueryBuilder) BuildKeysOnly(ctx context.Context) *datastore.Query {
	q := datastore.NewQuery(RunCLKind).Eq("IndexedID", b.CLID).KeysOnly(true)

	if b.Limit > 0 {
		q = q.Limit(b.Limit)
	}

	min := string(b.MinExcl)
	max := string(b.MaxExcl)
	if b.Project != "" {
		prMin, prMax := rangeOfProjectIDs(b.Project)
		if min == "" || min < prMin {
			min = prMin
		}
		if max == "" || max > prMax {
			max = prMax
		}
	}
	if min != "" {
		q = q.Gt("__key__", datastore.MakeKey(ctx, RunKind, min, RunCLKind, int64(b.CLID)))
	}
	if max != "" {
		q = q.Lt("__key__", datastore.MakeKey(ctx, RunKind, max, RunCLKind, int64(b.CLID)))
	}

	return q
}

// GetAllRunKeys runs the query across all matched RunCLs entities and returns
// Datastore keys to corresponding Run entities.
func (b CLQueryBuilder) GetAllRunKeys(ctx context.Context) ([]*datastore.Key, error) {
	// Fetch RunCL keys.
	var keys []*datastore.Key
	if err := datastore.GetAll(ctx, b.BuildKeysOnly(ctx), &keys); err != nil {
		return nil, errors.Annotate(err, "failed to fetch RunCLs IDs").Tag(transient.Tag).Err()
	}

	// Replace each RunCL key with its parent (Run) key.
	for i := range keys {
		keys[i] = keys[i].Parent()
	}
	return keys, nil
}

// LoadRuns returns matched Runs and the page token to continue search later.
func (b CLQueryBuilder) LoadRuns(ctx context.Context, checkers ...LoadRunChecker) ([]*Run, *PageToken, error) {
	return loadRunsFromQuery(ctx, b, checkers...)
}

// qLimit implements runKeysQuery interface.
func (b CLQueryBuilder) qLimit() int32 { return b.Limit }

// qPageToken implements runKeysQuery interface.
func (b CLQueryBuilder) qPageToken(pt *PageToken) runKeysQuery { return b.PageToken(pt) }

// ProjectQueryBuilder builds datastore.Query for searching Runs scoped to a
// LUCI project.
type ProjectQueryBuilder struct {
	// Project is the LUCI project. Required.
	Project string
	// Status optionally restricts query to Runs with this status.
	Status Status
	// MaxExcl restricts query to Runs with ID lexicographically smaller. Optional.
	//
	// This means query is restricted to Runs created after this Run.
	//
	// This Run must belong to the same LUCI project.
	MaxExcl common.RunID
	// MinExcl restricts query to Runs with ID lexicographically larger. Optional.
	//
	// This means query is restricted to Runs created before this Run.
	//
	// This Run must belong to the same LUCI project.
	MinExcl common.RunID

	// Limit limits the number of results if positive. Ignored otherwise.
	Limit int32
}

// isSatisfied returns whether the given Run satisfies the query.
func (b ProjectQueryBuilder) isSatisfied(r *Run) bool {
	switch {
	case r == nil:
	case r.ID.LUCIProject() != b.Project:
	case b.Status == Status_ENDED_MASK && !IsEnded(r.Status):
	case b.Status != Status_ENDED_MASK && b.Status != Status_STATUS_UNSPECIFIED && r.Status != b.Status:
	case b.MinExcl != "" && r.ID <= b.MinExcl:
	case b.MaxExcl != "" && r.ID >= b.MaxExcl:
	default:
		return true
	}
	return false
}

// After restricts the query to Runs created after the given Run.
//
// Panics if ProjectQueryBuilder is already constrained to a different Project.
func (b ProjectQueryBuilder) After(id common.RunID) ProjectQueryBuilder {
	if p := id.LUCIProject(); p != b.Project {
		if b.Project != "" {
			panic(fmt.Errorf("invalid ProjectQueryBuilder.After(%q): .Project is already set to %q", id, b.Project))
		}
		b.Project = p
	}
	b.MaxExcl = id
	return b
}

// Before restricts the query to Runs created before the given Run.
//
// Panics if ProjectQueryBuilder is already constrained to a different Project.
func (b ProjectQueryBuilder) Before(id common.RunID) ProjectQueryBuilder {
	if p := id.LUCIProject(); p != b.Project {
		if b.Project != "" {
			panic(fmt.Errorf("invalid ProjectQueryBuilder.Before(%q): .Project is already set to %q", id, b.Project))
		}
		b.Project = p
	}
	b.MinExcl = id
	return b
}

// PageToken constraints ProjectQueryBuilder to continue searching from the
// prior search.
func (b ProjectQueryBuilder) PageToken(pt *PageToken) ProjectQueryBuilder {
	if pt != nil {
		b.MinExcl = common.RunID(pt.GetRun())
	}
	return b
}

// BuildKeysOnly returns keys-only query on Run entities.
//
// It's exposed primarily for debugging reasons.
//
// WARNING: panics if Status is magic Status_ENDED_MASK,
// as it's not feasible to perform this as 1 query.
func (b ProjectQueryBuilder) BuildKeysOnly(ctx context.Context) *datastore.Query {
	q := datastore.NewQuery(RunKind).KeysOnly(true)

	switch b.Status {
	case Status_ENDED_MASK:
		panic(fmt.Errorf("Status=Status_ENDED_MASK is not yet supported"))
	case Status_STATUS_UNSPECIFIED:
	default:
		q = q.Eq("Status", int(b.Status))
	}

	if b.Limit > 0 {
		q = q.Limit(b.Limit)
	}

	if b.Project == "" {
		panic(fmt.Errorf("Project is not set"))
	}
	min, max := rangeOfProjectIDs(b.Project)

	switch {
	case b.MinExcl == "":
	case b.MinExcl.LUCIProject() != b.Project:
		panic(fmt.Errorf("MinExcl %q doesn't match Project %q", b.MinExcl, b.Project))
	default:
		min = string(b.MinExcl)
	}
	q = q.Gt("__key__", datastore.MakeKey(ctx, RunKind, min))

	switch {
	case b.MaxExcl == "":
	case b.MaxExcl.LUCIProject() != b.Project:
		panic(fmt.Errorf("MaxExcl %q doesn't match Project %q", b.MaxExcl, b.Project))
	default:
		max = string(b.MaxExcl)
	}
	q = q.Lt("__key__", datastore.MakeKey(ctx, RunKind, max))

	return q
}

// GetAllRunKeys runs the query and returns Datastore keys to Run entities.
func (b ProjectQueryBuilder) GetAllRunKeys(ctx context.Context) ([]*datastore.Key, error) {
	var keys []*datastore.Key

	if b.Status != Status_ENDED_MASK {
		if err := datastore.GetAll(ctx, b.BuildKeysOnly(ctx), &keys); err != nil {
			return nil, errors.Annotate(err, "failed to fetch Runs IDs").Tag(transient.Tag).Err()
		}
		return keys, nil
	}

	// Status_ENDED_MASK requires several dedicated queries.
	queries := make([]*datastore.Query, len(finalStatuses))
	for i, s := range finalStatuses {
		cpy := b
		cpy.Status = s
		queries[i] = cpy.BuildKeysOnly(ctx)
	}
	err := datastore.RunMulti(ctx, queries, func(k *datastore.Key) error {
		keys = append(keys, k)
		if b.Limit > 0 && len(keys) == int(b.Limit) {
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch Runs IDs").Tag(transient.Tag).Err()
	}
	return keys, err
}

// LoadRuns returns matched Runs and the page token to continue search later.
func (b ProjectQueryBuilder) LoadRuns(ctx context.Context, checkers ...LoadRunChecker) ([]*Run, *PageToken, error) {
	return loadRunsFromQuery(ctx, b, checkers...)
}

// qLimit implements runKeysQuery interface.
func (b ProjectQueryBuilder) qLimit() int32 { return b.Limit }

// qPageToken implements runKeysQuery interface.
func (b ProjectQueryBuilder) qPageToken(pt *PageToken) runKeysQuery { return b.PageToken(pt) }

// RecentQueryBuilder builds a VERY SLOW query for searching recent Runs
// across all LUCI projects.
//
// If two runs have the same timestamp, orders Runs first by the LUCI Project
// name and then by the remainder of the Run's ID.
//
// Beware: two Runs having the same timestamp is actually quite likely with Google
// Gerrit because it rounds updates to second granularity, which then makes its
// way as Run Creation time.
//
// **WARNING**: this is the most inefficient way to be used infrequently for CV
// admin needs only. Behind the scenes, it issues a Datastore query per active
// LUCI project.
//
// Doesn't yet support restricting search to a specific time range, but it can
// be easily implemented if necessary.
type RecentQueryBuilder struct {
	// Status optionally restricts query to Runs with this status.
	Status Status
	// Limit limits the number of results if positive. Ignored otherwise.
	Limit int32

	// lastFoundRunID is basically a page token.
	lastFoundRunID common.RunID
	// boundaryInverseTS / boundaryProject are just caches based on
	// lastFoundRunID set by PageToken(). See keysForProject() for their use.
	boundaryInverseTS string
	boundaryProject   string
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
// creation time across LUCI projects. So, the brute force is to query each known
// to CV LUCI project for most recent Run IDs, and then merge and select the
// next page of resulting keys.
func (b RecentQueryBuilder) GetAllRunKeys(ctx context.Context) ([]*datastore.Key, error) {
	// Load all enabled & disabled projects.
	projects, err := prjcfg.GetAllProjectIDs(ctx, false)
	if err != nil {
		return nil, err
	}

	// Do a query per project, in parallel.
	// KeysOnly queries are cheap in both time and money.
	allKeys := make([][]*datastore.Key, len(projects))
	errs := parallel.WorkPool(min(16, len(projects)), func(work chan<- func() error) {
		for i, p := range projects {
			i, p := i, p
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
func (b RecentQueryBuilder) LoadRuns(ctx context.Context, checkers ...LoadRunChecker) ([]*Run, *PageToken, error) {
	return loadRunsFromQuery(ctx, b, checkers...)
}

// qLimit implements runKeysQuery interface.
func (b RecentQueryBuilder) qLimit() int32 { return b.Limit }

// qPageToken implements runKeysQuery interface.
func (b RecentQueryBuilder) qPageToken(pt *PageToken) runKeysQuery { return b.PageToken(pt) }

// isSatisfied returns whether the given Run satisfies the query.
func (b RecentQueryBuilder) isSatisfied(r *Run) bool {
	switch {
	case r == nil:
	case b.Status == Status_ENDED_MASK && !IsEnded(r.Status):
	case b.Status != Status_ENDED_MASK && b.Status != Status_STATUS_UNSPECIFIED && r.Status != b.Status:
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
//  * DESC Created (== ASC InverseTS, or latest first)
//  * ASC  Project
//  * ASC  RunID (the remaining part of RunID)
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

// rangeOfProjectIDs returns (min..max) non-existent Run IDs, such that
// the following
//     min < $RunID  < max
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
	isSatisfied(*Run) bool
	qPageToken(*PageToken) runKeysQuery // must return a copy
	qLimit() int32
}

// loadRunsFromQuery returns matched Runs and a page token.
//
// If limit is specified, continues loading Runs until the limit is reached.
func loadRunsFromQuery(ctx context.Context, q runKeysQuery, checkers ...LoadRunChecker) ([]*Run, *PageToken, error) {
	if l := len(checkers); l > 1 {
		panic(fmt.Errorf("at most 1 LoadRunChecker allowed, %d given", l))
	}

	var out []*Run

	// Loop until we fetch the limit.
	// If `checkers` somehow filter out most Runs, e.g. because the
	// user can't see them, then this can loop for a very long time.
	limit := int(q.qLimit())
	for {
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

		loader := LoadRunsFromKeys(keys...)
		if len(checkers) == 1 {
			loader = loader.Checker(checkers[0])
		}
		runs, err := loader.DoIgnoreNotFound(ctx)
		if err != nil {
			return nil, nil, err
		}
		// Even for queries which can do everything using native Datastore query,
		// there is a window of time bewteen the Datastore query fetching keys of
		// satisfying Runs and actual Runs being fetched.
		// During this window, the Runs ultimately fetched could have been modified,
		// so check again and skip all fetched Runs which no longer satisfy the query.
		for _, r := range runs {
			if q.isSatisfied(r) {
				out = append(out, r)
			}
		}

		switch {
		case limit <= 0:
			return out, nil, nil

		case len(out) < limit:
			// Continue iterating, but start from the last considered key.
			q = q.qPageToken(&PageToken{Run: keys[len(keys)-1].StringID()})

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
			// NOTE: the keys themselves aren't necessarily ordered by ASC Run ID,
			// e.g. in case of RecentQueryBuilder. Therefore, we must iterate them in
			// order and can't do binary search.
			for i, k := range keys {
				if k.StringID() == firstNotReturnedID {
					return out[:limit], &PageToken{Run: keys[i-1].StringID()}, nil
				}
			}
			panic("unrechable")
		}
	}
}
