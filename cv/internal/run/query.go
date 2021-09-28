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
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

// NewQueryWithLUCIProject returns a query for Run entities that belongs
// to the given LUCI Project.
func NewQueryWithLUCIProject(ctx context.Context, project string) *datastore.Query {
	min, max := rangeOfProjectIDs(project)
	return datastore.NewQuery(RunKind).
		Gt("__key__", datastore.MakeKey(ctx, RunKind, min)).
		Lt("__key__", datastore.MakeKey(ctx, RunKind, max))
}

// CLQueryBuilder builds datastore.Query for searching Runs of a given CL.
type CLQueryBuilder struct {
	// CLID of the CL being searched for. Required.
	CLID common.CLID
	// Project optionally restricts Runs to the given LUCI project.
	Project string
	// Max restricts query to Runs with ID lexicographically smaller.
	//
	// This means query will return union of:
	//   * all Runs created after this Run in the same project,
	//   * all Runs in lexicographically smaller projects,
	//     unless .Project is set to the same project (recommended).
	Max common.RunID
	// Min restricts query to Runs with ID lexicographically larger.
	//
	// This means query will return union of:
	//   * all Runs created before this Run in the same project,
	//   * all Runs in lexicographically larger projects,
	//     unless .Project is set to the same project (recommended).
	Min common.RunID

	// Limit limits the number of results if positive. Ignored otherwise.
	Limit int32

	// Descending orders Run IDs from lexicographically largest to smallest.
	//
	// When also scoped to a single LUCI project, this orders Runs from oldest to
	// newest by creation time.
	Descending bool
}

// AfterInProject constrains CLQueryBuilder to Runs created after this Run but
// belonging to the same LUCI project.
//
// Typically used with the `Descending: true` to find the earliest Runs created
// after the given Run. For example, this will return the chronologically next
// Run in the same project:
//   CLQueryBuilder{CLID: cl, Descending: true, Limit: 1}.AfterInProject(run)
//
// Panics if CLQueryBuilder is already constrained to a different LUCI Project.
func (b CLQueryBuilder) AfterInProject(id common.RunID) CLQueryBuilder {
	if p := id.LUCIProject(); p != b.Project {
		if b.Project != "" {
			panic(fmt.Errorf("invalid CLQueryBuilder.After(%q): .Project is already set to %q", id, b.Project))
		}
		b.Project = p
	}
	b.Max = id
	return b
}

// BeforeInProject constrains CLQueryBuilder to Runs created before this Run but
// belonging to the same LUCI project.
//
// Panics if CLQueryBuilder is already constrained to a different LUCI Project.
func (b CLQueryBuilder) BeforeInProject(id common.RunID) CLQueryBuilder {
	if p := id.LUCIProject(); p != b.Project {
		if b.Project != "" {
			panic(fmt.Errorf("invalid CLQueryBuilder.Before(%q): .Project is already set to %q", id, b.Project))
		}
		b.Project = p
	}
	b.Min = id
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

	min := string(b.Min)
	max := string(b.Max)
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

	if b.Descending {
		q = q.Order("-__key__")
	}

	return q
}

func (b CLQueryBuilder) GetAllRunKeys(ctx context.Context) ([]*datastore.Key, error) {
	// Fetch RunCL keys.
	var keys []*datastore.Key
	if err := datastore.GetAll(ctx, b.BuildKeysOnly(ctx), &keys); err != nil {
		return nil, errors.Annotate(err, "failed to fetch Runs IDs").Tag(transient.Tag).Err()
	}

	// Replace each RunCL key with its parent (Run) key.
	for i := range keys {
		keys[i] = keys[i].Parent()
	}
	return keys, nil
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
