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
	execState, stateVer, err := tryjob.LoadExecutionState(ctx, r.ID)
	switch {
	case err != nil:
		return err
	case execState == nil:
		execState = initExecutionState()
	}

	plan, err := prepExecutionPlan(ctx, execState, r, payload.GetTryjobsUpdated(), payload.GetRequirementChanged())
	if err != nil {
		return err
	}
	sideEffect, err := e.executePlan(ctx, plan, r, execState)
	if err != nil {
		return err
	}
	var innerErr error
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		if err := tryjob.SaveExecutionState(ctx, r.ID, execState, stateVer); err != nil {
			return err
		}
		if sideEffect != nil {
			return sideEffect(ctx)
		}
		return nil
	}, nil)

	switch {
	case innerErr != nil:
		return innerErr
	case err != nil:
		return errors.Annotate(err, "failed to commit transaction").Tag(transient.Tag).Err()
	}
	return nil
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
				definition: definition,
				execution:  exec,
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
// Returns side effect that should be executed in the same transaction to save
// the new state.
func (e *Executor) executePlan(ctx context.Context, p *plan, r *run.Run, execState *tryjob.ExecutionState) (func(context.Context) error, error) {
	if p.isEmpty() {
		return nil, nil
	}
	var sideEffect func(context.Context) error
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
		var endedTryjobs []*tryjob.Tryjob
		for i, tj := range tryjobs {
			exec := executions[i]
			attempt := convertToAttempt(tj, r.ID)
			exec.Attempts = append(exec.Attempts, attempt)
			def := definitions[i]
			switch status := attempt.Status; {
			case status == tryjob.Status_ENDED:
				endedTryjobs = append(endedTryjobs, tj)
			case status == tryjob.Status_UNTRIGGERED && def.Critical:
				if criticalLaunchFailures == nil {
					criticalLaunchFailures = make(map[*tryjob.Definition]string)
				}
				criticalLaunchFailures[def] = tj.UntriggeredReason
			}
		}

		if len(criticalLaunchFailures) > 0 {
			execState.Status = tryjob.ExecutionState_FAILED
			execState.FailureReason = composeLaunchFailureReason(criticalLaunchFailures)
			return nil, nil
		}
		if len(endedTryjobs) > 0 {
			// For simplification, notify all Runs (including this run) about the
			// ended Tryjobs rather than processing it immediately.
			sideEffect = makeNotifyTryjobsUpdatedFn(ctx, endedTryjobs, e.RM)
		}
	}
	return sideEffect, nil
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

// makeNotifyTryjobsUpdatedFn computes the runs interested in the given
// tryjobs and returns a closure that notifies the runs about updates on
// their interested tryjobs.
func makeNotifyTryjobsUpdatedFn(ctx context.Context, tryjobs []*tryjob.Tryjob, rm rm) func(context.Context) error {
	notifyTargets := make(map[common.RunID]common.TryjobIDs)
	addToNotifyTargets := func(runID common.RunID, tjID common.TryjobID) {
		if _, ok := notifyTargets[runID]; !ok {
			notifyTargets[runID] = make(common.TryjobIDs, 0, 1)
		}
		notifyTargets[runID] = append(notifyTargets[runID], tjID)
	}
	for _, tj := range tryjobs {
		if tj.TriggeredBy != "" {
			addToNotifyTargets(tj.TriggeredBy, tj.ID)
		}
		for _, reuseRun := range tj.ReusedBy {
			addToNotifyTargets(reuseRun, tj.ID)
		}
	}

	return func(ctx context.Context) error {
		for run, tjIDs := range notifyTargets {
			events := &tryjob.TryjobUpdatedEvents{
				Events: make([]*tryjob.TryjobUpdatedEvent, tjIDs.Len()),
			}
			for i, tjID := range tjIDs {
				events.Events[i] = &tryjob.TryjobUpdatedEvent{
					TryjobId: int64(tjID),
				}
			}
			if err := rm.NotifyTryjobsUpdated(ctx, run, events); err != nil {
				return errors.Annotate(err, "failed to notify tryjobs updated").Tag(transient.Tag).Err()
			}
		}
		return nil
	}
}
