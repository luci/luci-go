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
	"go.chromium.org/luci/cv/internal/tryjob/requirement"
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
func Do(ctx context.Context, r *run.Run, t *tryjob.ExecuteTryjobsPayload, shouldStop func() bool) (*tryjob.ExecuteTryjobsResult, error) {
	execState, stateVer, err := loadExecutionState(ctx, r.ID)
	switch {
	case err != nil:
		return nil, err
	case execState == nil:
		execState = initExecutionState()
	}

	plan, err := prepExecutionPlan(ctx, execState, r, t.GetTryjobsUpdated(), t.GetRequirementChanged())
	if err != nil {
		return nil, err
	}
	result, err := plan.execute(ctx, r, execState)
	if err != nil {
		return nil, err
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
		return nil, innerErr
	case err != nil:
		return nil, errors.Annotate(err, "failed to commit transaction").Tag(transient.Tag).Err()
	default:
		return result, nil
	}
}

func initExecutionState() *tryjob.ExecutionState {
	return &tryjob.ExecutionState{
		Status: tryjob.ExecutionState_RUNNING,
	}
}

type planItem struct {
	defintion     *tryjob.Definition
	execution     *tryjob.ExecutionState_Execution
	discardReason string // Should be set only for discarded tryjobs.
}

type plan struct {
	// Trigger a new attempt for provided execution.
	//
	// Typically due to new tryjob definition or retrying failed tryjobs.
	triggerNewAttempt []planItem

	// Discard the tryjob execution is no longer needed.
	//
	// Typically due to tryjob definition is removed in Project Config.
	discard []planItem
}

func (p *plan) isEmpty() bool {
	return p == nil || (len(p.triggerNewAttempt)+len(p.discard) == 0)
}

const noLongerRequiredInConfig = "Tryjob is no longer required in Project Config"

// prepExecutionPlan updates the states and computes a plan in order to
// fulfill the requirement.
func prepExecutionPlan(ctx context.Context, execState *tryjob.ExecutionState, r *run.Run, tryjobsUpdated []int64, reqmtChanged bool) (*plan, error) {
	switch {
	case len(tryjobsUpdated) > 0 && reqmtChanged:
		panic(fmt.Errorf("the executor can't handle requirement update and tryjobs update at the same time"))
	case len(tryjobsUpdated) > 0:
		tryjobsByID, err := tryjob.LoadTryjobsMapByIDs(ctx, common.MakeTryjobIDs(tryjobsUpdated...))
		if err != nil {
			return nil, err
		}
		updateAttempts(execState, tryjobsByID)
		if retry := computeRetries(ctx, execState, tryjobsByID); len(retry) > 0 {
			return &plan{
				triggerNewAttempt: retry,
			}, nil
		}
		return nil, nil
	case reqmtChanged && execState.Status != tryjob.ExecutionState_RUNNING:
		// Execution has ended already. No need to react to requirement change.
		logging.Warningf(ctx, "Got requirement changed when execution is no longer running")
		return nil, nil
	case reqmtChanged:
		curReqmt, targetReqmt := execState.GetRequirement(), r.Tryjobs.GetRequirement()
		if targetReqmt.GetVersion() <= curReqmt.GetVersion() {
			logging.Errorf(ctx, "Tryjob executor is executing requirement that is either later or equal to the requested requirement version. current: %d, got: %d ", targetReqmt.GetVersion(), curReqmt.GetVersion())
			return nil, nil
		}
		existingExecutionByDef := make(map[*tryjob.Definition]*tryjob.ExecutionState_Execution, len(curReqmt.GetDefinitions()))
		for i, def := range curReqmt.GetDefinitions() {
			existingExecutionByDef[def] = execState.GetExecutions()[i]
		}
		reqmtDiff := requirement.Diff(curReqmt, targetReqmt)
		ret := &plan{}
		// populate discarded tryjob.
		for def := range reqmtDiff.RemovedDefs {
			ret.discard = append(ret.discard, planItem{
				defintion:     def,
				execution:     existingExecutionByDef[def],
				discardReason: noLongerRequiredInConfig,
			})
		}
		// Apply new requirement and figure out which execution requires triggering
		// new attempt.
		execState.Requirement = targetReqmt
		execState.Executions = execState.Executions[:0]
		for _, def := range targetReqmt.GetDefinitions() {
			switch {
			case reqmtDiff.AddedDefs.Has(def):
				exec := &tryjob.ExecutionState_Execution{}
				execState.Executions = append(execState.Executions, exec)
				ret.triggerNewAttempt = append(ret.triggerNewAttempt, planItem{
					defintion: def,
					execution: exec,
				})
			case reqmtDiff.ChangedDefsReverse.Has(def):
				existingDef := reqmtDiff.ChangedDefsReverse[def]
				exec, ok := existingExecutionByDef[existingDef]
				if !ok {
					panic(fmt.Errorf("impossible; definition shows up in diff but not in existing execution state. Definition: %s", existingDef))
				}
				execState.Executions = append(execState.Executions, exec)
			default: // Nothing has changed for this definition.
				exec, ok := existingExecutionByDef[def]
				if !ok {
					panic(fmt.Errorf("impossible; definition neither shows up in diff nor in existing execution state. Definition: %s", def))
				}
				execState.Executions = append(execState.Executions, exec)
			}
		}
		return ret, nil
	default:
		panic(fmt.Errorf("the executor is called without any update on either tryjobs or requirement"))
	}
}

// updateAttempts updates the attempts associated with the given tryjobs.
func updateAttempts(execState *tryjob.ExecutionState, tryjobsToUpdateByIDs map[common.TryjobID]*tryjob.Tryjob) {
	for _, exec := range execState.Executions {
		if len(exec.Attempts) == 0 {
			continue
		}
		// Only look at the most recent attempt in the execution.
		lastAttempt := exec.Attempts[len(exec.Attempts)-1]
		if tj, present := tryjobsToUpdateByIDs[common.TryjobID(lastAttempt.TryjobId)]; present {
			lastAttempt.Result = tj.Result
			lastAttempt.Status = tj.Status
		}
	}
}

// execute executes the plan and mutate the state.
func (p plan) execute(ctx context.Context, r *run.Run, execState *tryjob.ExecutionState) (*tryjob.ExecuteTryjobsResult, error) {
	panic("implement")
}
