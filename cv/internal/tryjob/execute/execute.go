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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
	"go.chromium.org/luci/cv/internal/tryjob/requirement"
)

// Executor reacts to changes in external world and tries to fulfills the tryjob
// requirement.
type Executor struct {
	// Backend is the Tryjob backend that executor will search reusable Tryjobs
	// from and launch new Tryjobs.
	Backend TryjobBackend
	// RM is used to notify Run for changes in Tryjob states.
	RM rm
	// ShouldStop is returns whether executor should stop the execution.
	ShouldStop func() bool
}

// TryjobBackend encapsulates the interactions with a Tryjob backend.
type TryjobBackend interface {
	Kind() string
	Search(ctx context.Context, cls []*run.RunCL, definitions []*tryjob.Definition, luciProject string, cb func(*tryjob.Tryjob) bool) error
	Launch(ctx context.Context, tryjobs []*tryjob.Tryjob, r *run.Run, cls []*run.RunCL) error
}

// TryjobBackend encapsulates the interactions with Run manager.
type rm interface {
	NotifyTryjobsUpdated(ctx context.Context, runID common.RunID, tryjobs *tryjob.TryjobUpdatedEvents) error
}

// Do executes the tryjob requirement for a run.
//
// This function is idempotent so it is safe to retry.
func (e *Executor) Do(ctx context.Context, r *run.Run, payload *tryjob.ExecuteTryjobsPayload) error {
	if !r.UseCVTryjobExecutor {
		logging.Warningf(ctx, "Tryjob executor is invoked when UseCVTryjobExecutor is set to false. It's likely caused by a race condition when reverting the migration config setting")
		return nil
	}

	execState, stateVer, err := tryjob.LoadExecutionState(ctx, r.ID)
	switch {
	case err != nil:
		return err
	case execState == nil:
		execState = initExecutionState()
	case execState.GetStatus() == tryjob.ExecutionState_SUCCEEDED:
		fallthrough
	case execState.GetStatus() == tryjob.ExecutionState_FAILED:
		logging.Warningf(ctx, "Tryjob executor is invoked after execution has ended")
		return nil
	}

	plan, err := prepExecutionPlan(ctx, execState, r, payload.GetTryjobsUpdated(), payload.GetRequirementChanged())
	switch {
	case err != nil:
		return err
	case plan.isEmpty():
		return nil
	}

	execState, err = e.executePlan(ctx, plan, r, execState)
	if err != nil {
		return err
	}
	var innerErr error
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		return tryjob.SaveExecutionState(ctx, r.ID, execState, stateVer)
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

func initExecutionState() *tryjob.ExecutionState {
	return &tryjob.ExecutionState{
		Status: tryjob.ExecutionState_RUNNING,
	}
}

type planItem struct {
	definition    *tryjob.Definition
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
	case reqmtChanged && r.Tryjobs.GetRequirementVersion() <= execState.GetRequirementVersion():
		logging.Errorf(ctx, "Tryjob executor is executing requirement that is either later or equal to the requested requirement version. current: %d, got: %d ", r.Tryjobs.GetRequirementVersion(), execState.GetRequirementVersion())
		return nil, nil
	case reqmtChanged:
		curReqmt, targetReqmt := execState.GetRequirement(), r.Tryjobs.GetRequirement()
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
			if !hasNonEndedCriticalTryjob {
				hasNonEndedCriticalTryjob = definition.GetCritical()
			}
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
		case attempt.Status == tryjob.Status_CANCELLED:
			// This SHOULD happen during race condition as a Tryjob is cancelled by
			// LUCI CV iff a new patchset is uploaded. In that case, Run SHOULD be
			// cancelled and try executor should never be invoked. Do nothing apart
			// from logging here and Run should be cancelled very soon.
			// TODO(yiwzhang): emit a log to record cancelled tryjobs.
		case attempt.Status == tryjob.Status_UNTRIGGERED && attempt.Reused:
			// Normally happens when this Run reuses a PENDING Tryjob from another
			// Run but the Tryjob doesn't end up getting triggered.
			ret.triggerNewAttempt = append(ret.triggerNewAttempt, planItem{
				definition: definition,
				execution:  exec,
			})
		case attempt.Status == tryjob.Status_UNTRIGGERED && !definition.GetCritical():
			// Ignore failure when launching non-critical tryjobs.
		case attempt.Status == tryjob.Status_UNTRIGGERED:
			// If a critical tryjob fails to launch, then this Run should have failed
			// already. So this code path should never be exercised.
			panic(fmt.Errorf("critical tryjob %d failed to launch but tryjob executor was asked to update this Tryjob", attempt.GetTryjobId()))
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
				definition: execState.GetRequirement().GetDefinitions()[idx],
				execution:  execState.GetExecutions()[idx],
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
		if attempt.Status != tj.Status || !proto.Equal(attempt.Result, tj.Result) {
			attempt.Status = tj.Status
			attempt.Result = tj.Result
			return true
		}
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
			definition:    def,
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
				definition: def,
				execution:  exec,
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

// executePlan executes the plan and mutate the state.
//
// Returns the mutated state.
func (e *Executor) executePlan(ctx context.Context, p *plan, r *run.Run, execState *tryjob.ExecutionState) (*tryjob.ExecutionState, error) {
	if len(p.discard) > 0 {
		// TODO(crbug/1323597): cancel the tryjobs because they are no longer
		// useful.
		// TODO(yiwzhang): emit a log that these tryjobs are discarded.
		for _, item := range p.discard {
			logging.Infof(ctx, "discard tryjob %s; reason: %s", item.definition, item.discardReason)
		}
	}

	if len(p.triggerNewAttempt) > 0 {
		definitions := make([]*tryjob.Definition, len(p.triggerNewAttempt))
		executions := make([]*tryjob.ExecutionState_Execution, len(p.triggerNewAttempt))
		for i, item := range p.triggerNewAttempt {
			definitions[i] = item.definition
			executions[i] = item.execution
		}

		tryjobs, err := e.startTryjobs(ctx, r, definitions, executions)
		if err != nil {
			return nil, err
		}

		var criticalLaunchFailures map[*tryjob.Definition]string
		for i, tj := range tryjobs {
			def, exec := definitions[i], executions[i]
			attempt := convertToAttempt(tj, r.ID)
			exec.Attempts = append(exec.Attempts, attempt)
			if attempt.Status == tryjob.Status_UNTRIGGERED && def.GetCritical() {
				if criticalLaunchFailures == nil {
					criticalLaunchFailures = make(map[*tryjob.Definition]string)
				}
				criticalLaunchFailures[def] = tj.UntriggeredReason
			}
		}

		if len(criticalLaunchFailures) > 0 {
			execState.Status = tryjob.ExecutionState_FAILED
			execState.FailureReason = composeLaunchFailureReason(criticalLaunchFailures)
			return execState, nil
		}
	}
	return execState, nil
}

func convertToAttempt(tj *tryjob.Tryjob, id common.RunID) *tryjob.ExecutionState_Execution_Attempt {
	ret := &tryjob.ExecutionState_Execution_Attempt{
		TryjobId:   int64(tj.ID),
		ExternalId: string(tj.ExternalID),
		Status:     tj.Status,
		Result:     tj.Result,
	}
	for _, runID := range tj.ReusedBy {
		if runID == id {
			ret.Reused = true
			break
		}
	}
	return ret
}
