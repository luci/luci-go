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
	var result *tryjob.ExecuteTryjobsResult
	if !plan.isEmpty() {
		var err error
		if result, err = plan.execute(ctx, r, execState); err != nil {
			return nil, err
		}
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
	switch hasTryjobsUpdated := len(tryjobsUpdated) > 0; {
	case hasTryjobsUpdated && reqmtChanged:
		panic(fmt.Errorf("the executor can't handle requirement update and tryjobs update at the same time"))
	case hasTryjobsUpdated:
		logging.Debugf(ctx, "received update for tryjobs %s", tryjobsUpdated)
		return handleUpdatedTryjobs(ctx, tryjobsUpdated, execState)
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
		return handleRequirementChange(curReqmt, targetReqmt, execState)
	default:
		panic(fmt.Errorf("the executor is called without any update on either tryjobs or requirement"))
	}
}

func handleUpdatedTryjobs(ctx context.Context, tryjobs []int64, execState *tryjob.ExecutionState) (*plan, error) {
	tryjobByID, err := tryjob.LoadTryjobsMapByIDs(ctx, common.MakeTryjobIDs(tryjobs...))
	if err != nil {
		return nil, err
	}
	var (
		ret                       plan
		failedIndices             []int            // critical tryjobs only
		failedTryjobs             []*tryjob.Tryjob // critical tryjobs only
		hasNonEndedCriticalTryjob bool
	)
	for i, exec := range execState.GetExecutions() {
		updated := updateLatestAttempt(exec, tryjobByID)
		definition := execState.GetRequirement().GetDefinitions()[i]
		switch latestAttemptEnded := hasLatestAttemptEnded(exec); {
		case !latestAttemptEnded:
			hasNonEndedCriticalTryjob = definition.GetCritical()
			continue
		case !updated:
			continue
		}
		// Only process the tryjob that has been updated and its latest attempt
		// has ended.
		switch attempt := exec.Attempts[len(exec.Attempts)-1]; {
		case attempt.Status == tryjob.Status_ENDED && attempt.GetResult().GetStatus() == tryjob.Result_SUCCEEDED:
			// TODO(yiwzhang): emit a log to record successful completion of this
			// tryjob.
		case attempt.Status == tryjob.Status_ENDED && definition.GetCritical():
			// TODO(yiwzhang): emit a log to record failed critical tryjob.
			failedIndices = append(failedIndices, i)
			failedTryjobs = append(failedTryjobs, tryjobByID[common.TryjobID(attempt.TryjobId)])
		case attempt.Status == tryjob.Status_ENDED:
			// TODO(yiwzhang): emit a log to record failed non critical tryjob.
		case attempt.Status == tryjob.Status_UNTRIGGERED:
			// Normally happens when this Run reuses a PENDING Tryjob from another
			// Run but the Tryjob doesn't end up getting triggered.
			ret.triggerNewAttempt = append(ret.triggerNewAttempt, planItem{
				defintion: definition,
				execution: exec,
			})
		default:
			panic(fmt.Errorf("unexpected attempt status %s for Tryjob %d", attempt.Status, attempt.TryjobId))
		}
	}

	switch hasFailed, hasNewAttemptToTrigger := len(failedIndices) > 0, len(ret.triggerNewAttempt) > 0; {
	case !hasFailed && !hasNewAttemptToTrigger && !hasNonEndedCriticalTryjob:
		// all critical tryjobs have completed successfully.
		execState.Status = tryjob.ExecutionState_SUCCEEDED
		return nil, nil
	case hasFailed:
		if ok, _ := canRetryAll(ctx, execState, failedIndices); !ok {
			// TODO(yiwzhang): log why execution can't be retried
			execState.Status = tryjob.ExecutionState_FAILED
			execState.FailureReason = composeReason(failedTryjobs)
			return nil, nil
		}
		for _, idx := range failedIndices {
			ret.triggerNewAttempt = append(ret.triggerNewAttempt, planItem{
				defintion: execState.GetRequirement().GetDefinitions()[idx],
				execution: execState.GetExecutions()[idx],
			})
		}
	}
	return &ret, nil
}

func updateLatestAttempt(exec *tryjob.ExecutionState_Execution, tryjobsByIDs map[common.TryjobID]*tryjob.Tryjob) bool {
	if len(exec.GetAttempts()) == 0 {
		return false
	}
	attempt := exec.Attempts[len(exec.Attempts)-1]
	if tj, ok := tryjobsByIDs[common.TryjobID(attempt.TryjobId)]; ok {
		attempt.Status = tj.Status
		attempt.Result = tj.Result
		return true
	}
	return false
}

func hasLatestAttemptEnded(exec *tryjob.ExecutionState_Execution) bool {
	if len(exec.GetAttempts()) == 0 {
		return false // hasn't launched any new attempt
	}
	// only look at the latest attempt
	switch latestAttempt := exec.GetAttempts()[len(exec.GetAttempts())-1]; latestAttempt.Status {
	case tryjob.Status_STATUS_UNSPECIFIED:
		panic(fmt.Errorf("attempt status not specified for Tryjob %d", latestAttempt.TryjobId))
	case tryjob.Status_PENDING, tryjob.Status_TRIGGERED:
		return false
	default:
		return true
	}
}

func handleRequirementChange(curReqmt, targetReqmt *tryjob.Requirement, execState *tryjob.ExecutionState) (*plan, error) {
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
}

// execute executes the plan and mutate the state.
func (p plan) execute(ctx context.Context, r *run.Run, execState *tryjob.ExecutionState) (*tryjob.ExecuteTryjobsResult, error) {
	panic("implement")
}
