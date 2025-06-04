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

var endedStatuses []run.Status

func init() {
	for s := range run.Status_name {
		status := run.Status(s)
		if status != run.Status_ENDED_MASK && run.IsEnded(status) {
			endedStatuses = append(endedStatuses, status)
		}
	}
}

// ProjectQueryBuilder builds datastore.Query for searching Runs scoped to a
// LUCI project.
type ProjectQueryBuilder struct {
	// Project is the LUCI project. Required.
	Project string
	// Status optionally restricts query to Runs with this status.
	Status run.Status
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
func (b ProjectQueryBuilder) isSatisfied(r *run.Run) bool {
	switch {
	case r == nil:
	case r.ID.LUCIProject() != b.Project:
	case b.Status == run.Status_ENDED_MASK && !run.IsEnded(r.Status):
	case b.Status != run.Status_ENDED_MASK && b.Status != run.Status_STATUS_UNSPECIFIED && r.Status != b.Status:
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
	q := datastore.NewQuery(common.RunKind).KeysOnly(true)

	switch b.Status {
	case run.Status_ENDED_MASK:
		panic(fmt.Errorf("Status=Status_ENDED_MASK is not yet supported"))
	case run.Status_STATUS_UNSPECIFIED:
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
	q = q.Gt("__key__", datastore.MakeKey(ctx, common.RunKind, min))

	switch {
	case b.MaxExcl == "":
	case b.MaxExcl.LUCIProject() != b.Project:
		panic(fmt.Errorf("MaxExcl %q doesn't match Project %q", b.MaxExcl, b.Project))
	default:
		max = string(b.MaxExcl)
	}
	q = q.Lt("__key__", datastore.MakeKey(ctx, common.RunKind, max))

	return q
}

// GetAllRunKeys runs the query and returns Datastore keys to Run entities.
func (b ProjectQueryBuilder) GetAllRunKeys(ctx context.Context) ([]*datastore.Key, error) {
	var keys []*datastore.Key

	if b.Status != run.Status_ENDED_MASK {
		if err := datastore.GetAll(ctx, b.BuildKeysOnly(ctx), &keys); err != nil {
			return nil, transient.Tag.Apply(errors.Fmt("failed to fetch Runs IDs: %w", err))
		}
		return keys, nil
	}

	// Status_ENDED_MASK requires several dedicated queries.
	queries := make([]*datastore.Query, len(endedStatuses))
	for i, s := range endedStatuses {
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
		return nil, transient.Tag.Apply(errors.Fmt("failed to fetch Runs IDs: %w", err))
	}
	return keys, err
}

// LoadRuns returns matched Runs and the page token to continue search later.
func (b ProjectQueryBuilder) LoadRuns(ctx context.Context, checkers ...run.LoadRunChecker) ([]*run.Run, *PageToken, error) {
	return loadRunsFromQuery(ctx, b, checkers...)
}

// qLimit implements runKeysQuery interface.
func (b ProjectQueryBuilder) qLimit() int32 { return b.Limit }

// qPageToken implements runKeysQuery interface.
func (b ProjectQueryBuilder) qPageToken(pt *PageToken) runKeysQuery { return b.PageToken(pt) }
