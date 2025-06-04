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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

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
		return nil, transient.Tag.Apply(errors.Fmt("failed to fetch Run: %w", err))
	}

	if err := checker.After(ctx, r); err != nil {
		return nil, err
	}

	return r, nil
}

// LoadRunsFromKeys prepares loading a Run for each given Datastore Key.
func LoadRunsFromKeys(keys ...*datastore.Key) LoadRunsBuilder {
	return LoadRunsBuilder{keys: keys}
}

// LoadRunsFromIDs prepares loading a Run for each given Run ID.
func LoadRunsFromIDs(ids ...common.RunID) LoadRunsBuilder {
	return LoadRunsBuilder{ids: ids}
}

// LoadRunsBuilder implements builder pattern for loading Runs.
type LoadRunsBuilder struct {
	// one of these must be set.
	ids  common.RunIDs
	keys []*datastore.Key

	checker LoadRunChecker
}

// Checker installs LoadRunChecker to perform checks before/after loading each
// Run, typically used for checking read permission.
func (b LoadRunsBuilder) Checker(c LoadRunChecker) LoadRunsBuilder {
	b.checker = c
	return b
}

// DoIgnoreNotFound loads and returns Runs in the same order as the input, but
// omitting not found ones.
//
// If used together with Checker:
//   - if Checker.Before returns error with NotFound code, treats such Run as
//     not found.
//   - if Run is not found in Datastore, Checker.After isn't called on it.
//   - if Checker.After returns error with NotFound code, treats such Run as
//     not found.
//
// Returns a singular first encountered error.
func (b LoadRunsBuilder) DoIgnoreNotFound(ctx context.Context) ([]*Run, error) {
	runs, errs := b.Do(ctx)
	out := runs[:0]
	for i, r := range runs {
		switch err := errs[i]; {
		case err == nil:
			out = append(out, r)
		case err == datastore.ErrNoSuchEntity:
			// Skip.
		default:
			if st, ok := appstatus.Get(err); !ok || st.Code() != codes.NotFound {
				return nil, err
			}
			// Also skip due to NotFound error.
		}
	}
	if len(out) == 0 {
		// Free memory immediately.
		return nil, nil
	}
	return out, nil
}

// Do loads Runs returning an error per each Run.
//
// If Run doesn't exist, the corresponding error is datastore.ErrNoSuchEntity or
// whatever Checker.After() returned if Checker is given.
//
// This is useful if you need to collate each loaded Run and its error with
// another original slice from which Run's keys or IDs were derived, e.g. an API
// request.
//
//	ids := make(common.RunIDs, len(batchReq))
//	for i, req := range batchReq {
//	  ids[i] = common.RunID(req.GetRunID())
//	}
//	runs, errs := run.LoadRunsFromIDs(ids...).Checker(acls.NewRunReadChecker()).Do(ctx)
//	respBatch := ...
//	for i := range ids {
//	  switch id, r, err := ids[i], runs[i], errs[i];{
//	  case err != nil:
//	    respBatch[i] = &respOne{Error: ...}
//	  default:
//	    respBatch[i] = &respOne{Run: ...}
//	  }
//	}
func (b LoadRunsBuilder) Do(ctx context.Context) ([]*Run, errors.MultiError) {
	loadFromDS := func(runs []*Run) errors.MultiError {
		totalErr := datastore.Get(ctx, runs)
		if totalErr == nil {
			return make(errors.MultiError, len(runs))
		}
		errs, ok := totalErr.(errors.MultiError)
		if !ok {
			// Assign the same error to each Run we tried to load.
			totalErr = transient.Tag.Apply(errors.Fmt("failed to load Runs: %w", totalErr))
			errs = make(errors.MultiError, len(runs))
			for i := range errs {
				errs[i] = totalErr
			}
			return errs
		}
		return errs
	}

	runs := b.prepareRunObjects()
	if b.checker == nil {
		// Without checker, can load all the Runs immediately.
		return runs, loadFromDS(runs)
	}

	// Call checker.Before() on each Run ID, recording non-nil errors and skipping
	// such Runs from the list of Runs to load.
	errs := make(errors.MultiError, len(runs))
	entities := make([]*Run, 0, len(runs))
	indexes := make([]int, 0, len(runs))
	for i, r := range runs {
		if err := b.checker.Before(ctx, r.ID); err != nil {
			errs[i] = err
		} else {
			entities = append(entities, r)
			indexes = append(indexes, i)
		}
	}

	loadErrs := loadFromDS(entities)
	for i, err := range loadErrs {
		switch {
		case err == nil:
			err = b.checker.After(ctx, entities[i])
		case err == datastore.ErrNoSuchEntity:
			err = b.checker.After(ctx, nil)
		}

		if err != nil {
			idx := indexes[i]
			errs[idx] = err
		}
	}
	return runs, errs
}

func (b LoadRunsBuilder) prepareRunObjects() []*Run {
	switch {
	case len(b.ids) > 0:
		out := make([]*Run, len(b.ids))
		for i, id := range b.ids {
			out[i] = &Run{ID: id}
		}
		return out
	case len(b.keys) > 0:
		out := make([]*Run, len(b.keys))
		for i, k := range b.keys {
			out[i] = &Run{ID: common.RunID(k.StringID())}
		}
		return out
	default:
		return nil
	}
}

// LoadRunCLs loads `RunCL` entities of the provided cls in the Run.
func LoadRunCLs(ctx context.Context, runID common.RunID, clids common.CLIDs) ([]*RunCL, error) {
	runCLs := make([]*RunCL, len(clids))
	runKey := datastore.MakeKey(ctx, common.RunKind, string(runID))
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
				return nil, errors.Fmt("RunCL %d not found in Datastore", runCLs[i].ID)
			}
		}
		count, err := merr.Summary()
		return nil, transient.Tag.Apply(errors.WrapIf(err, "failed to load %d out of %d RunCLs", count, len(runCLs)))
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to load %d RunCLs: %w", len(runCLs), err))
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
	runKey := datastore.MakeKey(ctx, common.RunKind, string(runID))
	q := datastore.NewQuery(RunLogKind).KeysOnly(true).Ancestor(runKey)
	if err := datastore.GetAll(ctx, q, &keys); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("failed to fetch keys of RunLog entities: %w", err))
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
		return nil, transient.Tag.Apply(errors.Fmt("failed to fetch RunLog entities: %w", common.MostSevereError(err)))
	}

	// Each RunLog entity contains at least 1 LogEntry.
	out := make([]*LogEntry, 0, len(entities))
	for _, e := range entities {
		out = append(out, e.Entries.GetEntries()...)
	}
	return out, nil
}

// LoadChildRuns loads all Runs with the given Run in their dep_runs.
func LoadChildRuns(ctx context.Context, runID common.RunID) ([]*Run, error) {
	q := datastore.NewQuery(common.RunKind).Eq("DepRuns", runID)
	var runs []*Run
	if err := datastore.GetAll(ctx, q, &runs); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("failed to fetch dependency Run entities: %w", err))
	}
	return runs, nil
}

type nullRunChecker struct{}

func (n nullRunChecker) Before(ctx context.Context, id common.RunID) error { return nil }
func (n nullRunChecker) After(ctx context.Context, runIfFound *Run) error  { return nil }
