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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

// CLQueryBuilder builds datastore.Query for searching Runs of a given CL.
type CLQueryBuilder struct {
	// CLID of the CL being searched for. Required.
	CLID common.CLID
	// Optional extra CLs that must be included.
	AdditionalCLIDs common.CLIDsSet
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
func (b CLQueryBuilder) isSatisfied(r *run.Run) bool {
	// If there are additional CLs that must be included,
	// check whether all of these are indeed included.
	if len(b.AdditionalCLIDs) > 0 {
		count := 0
		for _, clid := range r.CLs {
			if b.AdditionalCLIDs.Has(clid) {
				count += 1
			}
		}
		if count != len(b.AdditionalCLIDs) {
			return false
		}
	}
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
	q := datastore.NewQuery(run.RunCLKind).Eq("IndexedID", b.CLID).KeysOnly(true)

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
		q = q.Gt("__key__", datastore.MakeKey(ctx, common.RunKind, min, run.RunCLKind, int64(b.CLID)))
	}
	if max != "" {
		q = q.Lt("__key__", datastore.MakeKey(ctx, common.RunKind, max, run.RunCLKind, int64(b.CLID)))
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
func (b CLQueryBuilder) LoadRuns(ctx context.Context, checkers ...run.LoadRunChecker) ([]*run.Run, *PageToken, error) {
	return loadRunsFromQuery(ctx, b, checkers...)
}

// qLimit implements runKeysQuery interface.
func (b CLQueryBuilder) qLimit() int32 { return b.Limit }

// qPageToken implements runKeysQuery interface.
func (b CLQueryBuilder) qPageToken(pt *PageToken) runKeysQuery { return b.PageToken(pt) }
