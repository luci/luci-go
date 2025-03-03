// Copyright 2022 The LUCI Authors.
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

package tryjob

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

// TryjobExecutionLogKind is the kind name of executionLog entity.
const TryjobExecutionLogKind = "TryjobExecutionLog"

// executionState is a Datastore model which stores an `ExecutionState`
// along with the associated metadata.
type executionState struct {
	_kind string         `gae:"$kind,TryjobExecutionState"`
	_id   int64          `gae:"$id,1"`
	Run   *datastore.Key `gae:"$parent"`
	// EVersion is the version of this state. Start with 1.
	EVersion int64 `gae:",noindex"`
	// UpdateTime is exact time of when the state was last updated.
	UpdateTime time.Time `gae:",noindex"`
	State      *ExecutionState
}

// executionLog is an immutable record for changes to Tryjob execution state.
type executionLog struct {
	_kind string `gae:"$kind,TryjobExecutionLog"`

	// ID is the value executionState.EVersion which was saved
	// transactionally with the creation of the ExecutionLog entity.
	//
	// Thus, ordering by ID (default Datastore ordering) will automatically
	// provide semantically chronological order.
	ID  int64          `gae:"$id"`
	Run *datastore.Key `gae:"$parent"`
	// Entries record what happened to the Run.
	//
	// Ordered from oldest to newest.
	Entries *ExecutionLogEntries
}

// LoadExecutionState loads the ExecutionState of the given Run.
//
// Returns nil state and 0 version if execution state never exists for this
// Run before.
func LoadExecutionState(ctx context.Context, rid common.RunID) (state *ExecutionState, version int64, err error) {
	es := &executionState{Run: datastore.MakeKey(ctx, common.RunKind, string(rid))}
	switch err := datastore.Get(ctx, es); {
	case err == datastore.ErrNoSuchEntity:
		return nil, 0, nil
	case err != nil:
		return nil, 0, errors.Annotate(err, "failed to load tryjob execution state of run %q", rid).Tag(transient.Tag).Err()
	default:
		return es.State, es.EVersion, nil
	}
}

// SaveExecutionState saves new ExecutionState and logs for the run.
//
// Fails if the current version of the state is different from the provided
// `expectedVersion`. This typically means the state has changed since the
// last `LoadState`. `expectedVersion==0` means the state never exists for
// this Run before.
//
// Must be called in a transaction.
func SaveExecutionState(ctx context.Context, rid common.RunID, state *ExecutionState, expectedVersion int64, logEntries []*ExecutionLogEntry) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic(fmt.Errorf("must be called in a transaction"))
	}
	switch _, latestStateVer, err := LoadExecutionState(ctx, rid); {
	case err != nil:
		return err
	case latestStateVer != expectedVersion:
		return errors.Reason("execution state has changed. before: %d, current: %d", expectedVersion, latestStateVer).Tag(transient.Tag).Err()
	default:
		runKey := datastore.MakeKey(ctx, common.RunKind, string(rid))
		newState := &executionState{
			Run:        runKey,
			EVersion:   latestStateVer + 1,
			UpdateTime: clock.Now(ctx).UTC(),
			State:      state,
		}

		if len(logEntries) == 0 {
			if err := datastore.Put(ctx, newState); err != nil {
				return errors.Annotate(err, "failed to save execution state").Tag(transient.Tag).Err()
			}
		} else {
			el := &executionLog{
				ID:  newState.EVersion,
				Run: runKey,
				Entries: &ExecutionLogEntries{
					Entries: logEntries,
				},
			}
			if err := datastore.Put(ctx, newState, el); err != nil {
				return errors.Annotate(err, "failed to save execution state and log").Tag(transient.Tag).Err()
			}
		}
		return nil
	}
}

// LoadExecutionLogs loads all the Tryjob execution log entries for a given Run.
//
// Ordered from logically oldest to newest as it assumes logs associated with
// a smaller ExecutionState EVersion should happen earlier than the logs
// associated with a larger ExecutionState EVersion.
func LoadExecutionLogs(ctx context.Context, runID common.RunID) ([]*ExecutionLogEntry, error) {
	var keys []*datastore.Key
	runKey := datastore.MakeKey(ctx, common.RunKind, string(runID))
	// Getting the key first as getting the execution log entity directly is
	// more likely a cache hit.
	q := datastore.NewQuery("TryjobExecutionLog").KeysOnly(true).Ancestor(runKey)
	if err := datastore.GetAll(ctx, q, &keys); err != nil {
		return nil, errors.Annotate(err, "failed to fetch keys of TryjobExecutionLog entities").Tag(transient.Tag).Err()
	}
	if len(keys) == 0 {
		return nil, nil
	}
	entities := make([]*executionLog, len(keys))
	for i, key := range keys {
		entities[i] = &executionLog{
			ID:  key.IntID(),
			Run: runKey,
		}
	}
	if err := datastore.Get(ctx, entities); err != nil {
		// It's possible to get EntityNotExists, but it may only happen if data
		// retention enforcement is deleting old entities at the same time.
		// Thus, treat all errors as transient.
		return nil, errors.Annotate(common.MostSevereError(err), "failed to fetch TryjobExecutionLog entities").Tag(transient.Tag).Err()
	}

	// Each TryjobExecutionLog entity contains at least 1 LogEntry.
	ret := make([]*ExecutionLogEntry, 0, len(entities))
	for _, e := range entities {
		ret = append(ret, e.Entries.GetEntries()...)
	}
	return ret, nil
}

// AreAllCriticalTryjobsEnded returns true if all critical tryjobs ended.
func AreAllCriticalTryjobsEnded(state *ExecutionState) bool {
	if state.GetStatus() == ExecutionState_SUCCEEDED {
		// No critical tryjobs should be running at this point.
		return true
	}
	for i, def := range state.GetRequirement().GetDefinitions() {
		attempt := LatestAttempt(state.GetExecutions()[i])
		if def.GetCritical() && !IsEnded(attempt.GetStatus()) {
			return false
		}
	}
	return true
}
