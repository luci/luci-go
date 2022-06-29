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

// LoadExecutionState loads the execution state of the given Run.
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

// SaveExecutionState saves new state for the run.
//
// Fails if the current version of the state is different from the provided
// `expectedVersion`. This typically means the state has changed since the
// last `LoadState`. `expectedVersion==0` means the state never exists for
// this Run before.
//
// Must be called in a transaction.
func SaveExecutionState(ctx context.Context, rid common.RunID, state *ExecutionState, expectedVersion int64) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic(fmt.Errorf("must be called in a transaction"))
	}
	switch _, latestStateVer, err := LoadExecutionState(ctx, rid); {
	case err != nil:
		return err
	case latestStateVer != expectedVersion:
		return errors.Reason("execution state has changed. before: %d, current: %d", expectedVersion, latestStateVer).Tag(transient.Tag).Err()
	default:
		newState := &executionState{
			Run:        datastore.MakeKey(ctx, common.RunKind, string(rid)),
			EVersion:   latestStateVer + 1,
			UpdateTime: clock.Now(ctx).UTC(),
			State:      state,
		}
		if err := datastore.Put(ctx, newState); err != nil {
			return errors.Annotate(err, "failed to save execution state").Tag(transient.Tag).Err()
		}
	}
	return nil
}
