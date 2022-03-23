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

package execute

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

type state struct {
	_kind string         `gae:"$kind,TryjobExecutionState"`
	_id   int64          `gae:"$id,1"`
	Run   *datastore.Key `gae:"$parent"`
	// EVersion is the version of this state. Start with 1.
	EVersion int64
	State    *tryjob.ExecutionState
}

func loadExecutionState(ctx context.Context, rid common.RunID) (*tryjob.ExecutionState, int64, error) {
	s := &state{Run: datastore.MakeKey(ctx, run.RunKind, string(rid))}
	switch err := datastore.Get(ctx, s); {
	case err == datastore.ErrNoSuchEntity:
		return nil, 0, nil
	case err != nil:
		return nil, 0, errors.Annotate(err, "failed to load tryjob execution state of run %q", rid).Err()
	default:
		return s.State, s.EVersion, nil
	}
}

// Do executes the tryjob requirement for a run.
//
// This function is idempotent so it is safe to retry.
func Do(ctx context.Context, r *run.Run, shouldStop func() bool) error {
	reqmt := r.Tryjobs.GetRequirement()
	switch {
	case reqmt == nil:
		return errors.New("tryjob executor is invoked without requirement")
	case len(reqmt.GetDefinitions()) == 0:
		return errors.New("tryjob executor is invoked without any definitions in requirement")
	}

	execState, stateVer, err := loadExecutionState(ctx, r.ID)
	switch {
	case err != nil:
		return err
	case execState.GetRequirement().GetVersion() > reqmt.GetVersion():
		panic(fmt.Errorf("impossible; tryjob executor is executing requirement with version %d which is larger than the requirement version in run %d",
			execState.GetRequirement().GetVersion(), reqmt.GetVersion()))
	case execState.GetRequirement().GetVersion() == reqmt.GetVersion():
		logging.Warningf(ctx, "tryjob executor is already executing requirement v%d which is current in run", reqmt.GetVersion())
		return nil
	case execState == nil:
		execState = &tryjob.ExecutionState{}
	}

	plan := prepExecutionPlan(reqmt, execState)
	if err := plan.execute(ctx, r, execState); err != nil {
		return err
	}

	var innerErr error
	err = datastore.RunInTransaction(ctx, func(c context.Context) (err error) {
		defer func() { innerErr = err }()
		switch _, latestStateVer, err := loadExecutionState(ctx, r.ID); {
		case err != nil:
			return err
		case latestStateVer != stateVer:
			return errors.Reason("execution state has changed. before: %d, current: %d", stateVer, latestStateVer).Tag(transient.Tag).Err()
		default:
			s := &state{
				Run:      datastore.MakeKey(ctx, run.RunKind, string(r.ID)),
				EVersion: latestStateVer + 1,
				State:    execState,
			}
			return errors.Annotate(datastore.Put(ctx, s), "failed to save execution state").Tag(transient.Tag).Err()
		}
	}, nil)

	switch {
	case innerErr != nil:
		return innerErr
	case err != nil:
		return errors.Annotate(err, "failed to commit transaction").Tag(transient.Tag).Err()
	default:
		return nil
	}
}

type plan struct {
	// New tryjob to launch.
	launchNew []*tryjob.Definition

	// The indices of the tryjob execution in the existing execution state that
	// should be discarded.
	//
	// This is typically due to the builder is deleted in the Project Config.
	discardIndices intSet
	// TODO(yiwzhang): Consider supporting the use case where a builder changes
	// `disable_reuse` from false to true, then if this builder is currently
	// reusing a build, it should stop reusing it and launch a new build.
}

func prepExecutionPlan(newReqmt *tryjob.Requirement, execState *tryjob.ExecutionState) plan {
	panic("implement")
}

// execute executes the plan and mutate the state.
func (p plan) execute(ctx context.Context, r *run.Run, execState *tryjob.ExecutionState) error {
	panic("implement")
}
