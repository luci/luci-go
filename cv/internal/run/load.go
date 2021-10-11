// Copyright 2021 The LUCI Authors.
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

// LoadRunChecker allows to plug ACL checking when loading Run from Datastore.
//
// See LoadRun().
type LoadRunChecker interface {
	// Before is called by LoadRun before attempting to load Run from Datastore.
	//
	// If Before returns an error, it's returned as is to the caller of LoadRun.
	Before(ctx context.Context, id common.RunID) error
	// After is called by LoadRun after loading Run from Datastore.
	//
	// If Run wasn't found, nil is passed.
	//
	// If After returns an error, it's returned as is to the caller of LoadRun.
	After(ctx context.Context, runIfFound *Run) error
}

// LoadRun returns Run from Datastore, optionally performing before/after
// checks, typically used for checking read permissions.
//
// If Run isn't found, returns (nil, nil), unless optional checker returns an
// error.
func LoadRun(ctx context.Context, id common.RunID, checkers ...LoadRunChecker) (*Run, error) {
	var checker LoadRunChecker = nullRunChecker{}
	switch l := len(checkers); {
	case l > 1:
		panic(fmt.Errorf("at most 1 LoadRunChecker allowed, %d given", l))
	case l == 1:
		checker = checkers[0]
	}

	if err := checker.Before(ctx, id); err != nil {
		return nil, err
	}

	r := &Run{ID: id}
	switch err := datastore.Get(ctx, r); {
	case err == datastore.ErrNoSuchEntity:
		r = nil
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Run").Tag(transient.Tag).Err()
	}

	if err := checker.After(ctx, r); err != nil {
		return nil, err
	}

	return r, nil
}

// LoadRunCLs loads `RunCL` entities of the provided cls in the Run.
func LoadRunCLs(ctx context.Context, runID common.RunID, clids common.CLIDs) ([]*RunCL, error) {
	runCLs := make([]*RunCL, len(clids))
	runKey := datastore.MakeKey(ctx, RunKind, string(runID))
	for i, clID := range clids {
		runCLs[i] = &RunCL{
			ID:  clID,
			Run: runKey,
		}
	}
	err := datastore.Get(ctx, runCLs)
	switch merr, ok := err.(errors.MultiError); {
	case ok:
		for i, err := range merr {
			if err == datastore.ErrNoSuchEntity {
				return nil, errors.Reason("RunCL %d not found in Datastore", runCLs[i].ID).Err()
			}
		}
		count, err := merr.Summary()
		return nil, errors.Annotate(err, "failed to load %d out of %d RunCLs", count, len(runCLs)).Tag(transient.Tag).Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to load %d RunCLs", len(runCLs)).Tag(transient.Tag).Err()
	}
	return runCLs, nil
}

// LoadRunLogEntries loads all log entries of a given Run.
//
// Ordered from logically oldest to newest.
func LoadRunLogEntries(ctx context.Context, runID common.RunID) ([]*LogEntry, error) {
	// Since RunLog entities are immutable, it's cheapest to load them from
	// DS cache. So, perform KeysOnly query first, which is cheap & fast, and then
	// additional multi-Get which will go via DS cache.

	var keys []*datastore.Key
	runKey := datastore.MakeKey(ctx, RunKind, string(runID))
	q := datastore.NewQuery(RunLogKind).KeysOnly(true).Ancestor(runKey)
	if err := datastore.GetAll(ctx, q, &keys); err != nil {
		return nil, errors.Annotate(err, "failed to fetch keys of RunLog entities").Tag(transient.Tag).Err()
	}

	entities := make([]*RunLog, len(keys))
	for i, key := range keys {
		entities[i] = &RunLog{
			Run: runKey,
			ID:  key.IntID(),
		}
	}
	if err := datastore.Get(ctx, entities); err != nil {
		// It's possible to get EntityNotExists, it may only happen if data
		// retention enforcement is deleting old entities at the same time.
		// Thus, treat all errors as transient.
		return nil, errors.Annotate(common.MostSevereError(err), "failed to fetch RunLog entities").Tag(transient.Tag).Err()
	}

	// Each RunLog entity contains at least 1 LogEntry.
	out := make([]*LogEntry, 0, len(entities))
	for _, e := range entities {
		out = append(out, e.Entries.GetEntries()...)
	}
	return out, nil
}

type nullRunChecker struct{}

func (n nullRunChecker) Before(ctx context.Context, id common.RunID) error { return nil }
func (n nullRunChecker) After(ctx context.Context, runIfFound *Run) error  { return nil }
