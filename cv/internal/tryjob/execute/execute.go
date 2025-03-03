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
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
	"go.chromium.org/luci/cv/internal/tryjob/requirement"
)

// Executor reacts to changes in the external world and tries to fulfill the
// Tryjob requirement.
type Executor struct {
	// Env is the environment that LUCI runs in.
	Env *common.Env
	// Backend is the Tryjob backend that Executor will search reusable Tryjobs
	// from and launch new Tryjobs.
	Backend TryjobBackend
	// RM is used to notify Run about changes in Tryjob states.
	RM rm
	// ShouldStop returns whether Executor should stop the execution.
	ShouldStop func() bool
	// logEntries records what has happened during the execution.
	logEntries []*tryjob.ExecutionLogEntry
	// stagedMetricReportFns temporally stores metrics reporting functions.
	//
	// Those functions will be called after the transaction to update the
	// execution state succeeds to ensure LUCI CV doesn't over-report when it
	// encounters failure and retries.
	stagedMetricReportFns []func(context.Context)
}

// TryjobBackend encapsulates the interactions with a Tryjob backend.
type TryjobBackend interface {
	Kind() string
	Search(ctx context.Context, cls []*run.RunCL, definitions []*tryjob.Definition, luciProject string, cb func(*tryjob.Tryjob) bool) error
	Launch(ctx context.Context, tryjobs []*tryjob.Tryjob, r *run.Run, cls []*run.RunCL) []*tryjob.LaunchResult
}

// TryjobBackend encapsulates the interactions with Run Manager.
type rm interface {
	NotifyTryjobsUpdated(ctx context.Context, runID common.RunID, tryjobs *tryjob.TryjobUpdatedEvents) error
}

// Do executes the Tryjob Requirement for a Run.
//
// This function is idempotent, so it is safe to retry.
func (e *Executor) Do(ctx context.Context, r *run.Run, payload *tryjob.ExecuteTryjobsPayload) error {
	// Clearing the stateful properties in case the same Executor instance gets
	// reused unexpectedly.
	e.logEntries = nil
	e.stagedMetricReportFns = nil

	execState, stateVer, err := tryjob.LoadExecutionState(ctx, r.ID)
	switch {
	case err != nil:
		return err
	case execState == nil:
		execState = initExecutionState()
	}

	execState, plan, err := e.prepExecutionPlan(ctx, execState, r, payload.GetTryjobsUpdated(), payload.GetRequirementChanged())
	switch {
	case err != nil:
		return err
	case !plan.isEmpty():
		execState, err = e.executePlan(ctx, plan, r, execState)
		if err != nil {
			return err
		}
	}

	// Record the execution end time.
	if execState.GetEndTime() == nil {
		switch execState.Status {
		case tryjob.ExecutionState_SUCCEEDED, tryjob.ExecutionState_FAILED:
			execState.EndTime = timestamppb.New(clock.Now(ctx).UTC())
		}
	}

	// TODO(yiwzhang): optimize the code to save the state when it is changed or
	// has new log entries. However, most of the time, the state will be changed.
	// Therefore, the cost is trivial to always save the state.
	var innerErr error
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		if err := tryjob.SaveExecutionState(ctx, r.ID, execState, stateVer, e.logEntries); err != nil {
			return err
		}
		if len(e.stagedMetricReportFns) > 0 {
			txndefer.Defer(ctx, func(ctx context.Context) {
				for _, reportFn := range e.stagedMetricReportFns {
					reportFn(ctx)
				}
			})
		}
		return nil
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

// planItem represents a thing to do for one Tryjob for a Run.
type planItem struct {
	definition    *tryjob.Definition
	execution     *tryjob.ExecutionState_Execution
	discardReason string // Should be set only for discarded tryjobs.
}

// plan represents a set of things to do for Tryjobs for a Run.
type plan struct {
	// Trigger a new attempt for provided execution.
	//
	// Typically used if to new tryjob definition or retrying failed tryjobs.
	triggerNewAttempt []planItem

	// Discard the tryjob execution as it is no longer needed.
	//
	// Typically used if the tryjob definition is removed from the Project Config.
	discard []planItem
}

// isEmpty checks whether the plan is nil or has no items.
func (p *plan) isEmpty() bool {
	return p == nil || (len(p.triggerNewAttempt)+len(p.discard) == 0)
}

const noLongerRequiredInConfig = "Tryjob is no longer required in Project Config"

// prepExecutionPlan updates the states and computes a plan in order to
// fulfill the Requirement.
func (e *Executor) prepExecutionPlan(ctx context.Context, execState *tryjob.ExecutionState, r *run.Run, tryjobsUpdated []int64, reqmtChanged bool) (*tryjob.ExecutionState, *plan, error) {
	switch hasTryjobsUpdated := len(tryjobsUpdated) > 0; {
	case hasTryjobsUpdated && reqmtChanged:
		panic(fmt.Errorf("the Executor can't handle updates to Requirement and Tryjobs at the same time"))
	case hasTryjobsUpdated && hasExecutionStateEnded(execState):
		logging.Debugf(ctx, "received update for tryjobs %v", tryjobsUpdated)
		// execution has ended. Only updating the tryjob information in the
		// execution state.
		execState, err := e.updateTryjobs(ctx, tryjobsUpdated, execState)
		if err != nil {
			return nil, nil, err
		}
		return execState, nil, nil
	case hasTryjobsUpdated:
		logging.Debugf(ctx, "received update for tryjobs %v", tryjobsUpdated)
		return e.handleUpdatedTryjobs(ctx, tryjobsUpdated, execState, r)
	case reqmtChanged && hasExecutionStateEnded(execState):
		logging.Infof(ctx, "Received requirement changes but the Executor has ended")
		return execState, nil, nil
	case reqmtChanged && r.Tryjobs.GetRequirementVersion() <= execState.GetRequirementVersion():
		logging.Errorf(ctx, "Tryjob Executor is executing a Requirement that is either later than or equal to the requested Requirement version. current: %d, got: %d ", r.Tryjobs.GetRequirementVersion(), execState.GetRequirementVersion())
		return execState, nil, nil
	case reqmtChanged:
		curReqmt, targetReqmt := execState.GetRequirement(), r.Tryjobs.GetRequirement()
		if curReqmt != nil {
			// Only log when existing Requirement is present. In another words,
			// the Requirement has changed when Run is running.
			e.logRequirementChanged(ctx)
		}
		return handleRequirementChange(curReqmt, targetReqmt, execState)
	default:
		panic(fmt.Errorf("the Executor was called without any update on either Tryjobs or Requirement"))
	}
}

// updateTryjobs updates the relevant tryjob in the execution state.
func (e *Executor) updateTryjobs(ctx context.Context, tryjobs []int64, execState *tryjob.ExecutionState) (*tryjob.ExecutionState, error) {
	updatedTryjobByID, err := tryjob.LoadTryjobsMapByIDs(ctx, common.MakeTryjobIDs(tryjobs...))
	if err != nil {
		return nil, err
	}
	var endedTryjobLogs []*tryjob.ExecutionLogEntry_TryjobSnapshot

	for i, exec := range execState.GetExecutions() {
		updated := updateAttempts(exec, updatedTryjobByID)
		if updated {
			endedTryjobLogs = append(endedTryjobLogs, makeLogTryjobSnapshotFromAttempt(execState.GetRequirement().GetDefinitions()[i], tryjob.LatestAttempt(exec)))
		}
	}
	if len(endedTryjobLogs) > 0 {
		e.log(&tryjob.ExecutionLogEntry{
			Time: timestamppb.New(clock.Now(ctx).UTC()),
			Kind: &tryjob.ExecutionLogEntry_TryjobsEnded_{
				TryjobsEnded: &tryjob.ExecutionLogEntry_TryjobsEnded{
					Tryjobs: endedTryjobLogs,
				},
			},
		})
	}
	return execState, nil
}

// handleUpdatedTryjobs updates the ExecutionState given the latest Tryjobs
// and computes the next steps (the plan) if any.
//
// Returns a ExecutionState with failed status and empty plan if any critical
// Tryjob(s) have failed and can not be retried.
// Returns a ExecutionState with succeeded status and empty plan if all
// critical Tryjobs have ended successfully.
// Returns a non-empty plan if any Tryjob should be retried.
func (e *Executor) handleUpdatedTryjobs(ctx context.Context, tryjobs []int64, execState *tryjob.ExecutionState, r *run.Run) (*tryjob.ExecutionState, *plan, error) {
	tryjobByID, err := tryjob.LoadTryjobsMapByIDs(ctx, common.MakeTryjobIDs(tryjobs...))
	if err != nil {
		return nil, nil, err
	}
	var (
		p                         plan
		failedIndices             []int            // Critical Tryjobs only
		failedTryjobs             []*tryjob.Tryjob // Critical Tryjobs only
		hasNonEndedCriticalTryjob bool
		endedTryjobLogs           []*tryjob.ExecutionLogEntry_TryjobSnapshot
	)
	for i, exec := range execState.GetExecutions() {
		latestAttemptUpdated := updateAttempts(exec, tryjobByID)
		attempt := tryjob.LatestAttempt(exec)
		definition := execState.GetRequirement().GetDefinitions()[i]
		hasNonEndedCriticalTryjob = hasNonEndedCriticalTryjob || (!hasAttemptEnded(attempt) && definition.GetCritical())
		if !latestAttemptUpdated || attempt == nil {
			continue // Only process the Tryjob when the latest attempt gets updated
		}

		switch attempt.Status {
		case tryjob.Status_STATUS_UNSPECIFIED:
			panic(fmt.Errorf("attempt status not specified"))
		case tryjob.Status_PENDING:
			// Nothing to do but waiting for the Tryjob getting launched.
		case tryjob.Status_TRIGGERED:
			// An ongoing reused Tryjob may tell CV that the mode of the current Run
			// is not allowed to reuse this Tryjob. In this case, trigger a new
			// Tryjob for this Run instead.
			// TODO(yiwzhang): Add a new execution log to explain why a new Tryjob
			// gets triggered.
			if output := attempt.GetResult().GetOutput(); attempt.GetReused() &&
				output != nil &&
				!isModeAllowed(r.Mode, output.GetReusability().GetModeAllowlist()) {
				p.triggerNewAttempt = append(p.triggerNewAttempt, planItem{
					definition: definition,
					execution:  exec,
				})
			}
		case tryjob.Status_ENDED:
			endedTryjobLogs = append(endedTryjobLogs, makeLogTryjobSnapshotFromAttempt(definition, attempt))
			switch {
			case !definition.GetCritical():
				// Don't waste time on the result status for non-critical Tryjob.
			case attempt.GetResult().GetStatus() != tryjob.Result_SUCCEEDED:
				failedIndices = append(failedIndices, i)
				failedTryjobs = append(failedTryjobs, tryjobByID[common.TryjobID(attempt.TryjobId)])
				// FIXME(yiwzhang): This is a bug that if a critical Tryjob fails and
				// this Tryjob is a reuse, CV should launch a new Tryjob for this
				// Run directly instead of going through the retry code path.
			case attempt.GetReused() && !isModeAllowed(r.Mode, attempt.GetResult().GetOutput().GetReusability().GetModeAllowlist()):
				// A reused Tryjob, even if it succeeds, could end with declaring
				// itself not reusable for the mode of the current Run. In this case,
				// Trigger a new Tryjob for this Run instead.
				// TODO(yiwzhang): Add a new execution log to explain why a new Tryjob
				// gets triggered.
				p.triggerNewAttempt = append(p.triggerNewAttempt, planItem{
					definition: definition,
					execution:  exec,
				})
			}
		case tryjob.Status_CANCELLED:
			// This SHOULD only happen during race condition as a Tryjob is
			// cancelled by LUCI CV iff a new patchset is uploaded. In that
			// case, Run SHOULD be cancelled and Tryjob Executor should never
			// be invoked. So, do nothing apart from logging here, as the Run
			// should be cancelled very soon.
			endedTryjobLogs = append(endedTryjobLogs, makeLogTryjobSnapshotFromAttempt(definition, attempt))
		case tryjob.Status_UNTRIGGERED:
			switch {
			case attempt.GetReused():
				// Normally happens when this Run reuses a PENDING Tryjob from
				// another Run but the Tryjob doesn't end up getting triggered.
				p.triggerNewAttempt = append(p.triggerNewAttempt, planItem{
					definition: definition,
					execution:  exec,
				})
			case !definition.GetCritical():
				// Ignore failure when launching non-critical Tryjobs.
			default:
				// If a critical Tryjob failed to launch, then this Run should have
				// failed already. So, this code path should never be exercised.
				panic(fmt.Errorf("critical tryjob %d failed to launch but tryjob executor was asked to update this Tryjob", attempt.GetTryjobId()))
			}
		default:
			panic(fmt.Errorf("unexpected attempt status %s for Tryjob %d", attempt.Status, attempt.TryjobId))
		}
	}

	if len(endedTryjobLogs) > 0 {
		e.log(&tryjob.ExecutionLogEntry{
			Time: timestamppb.New(clock.Now(ctx).UTC()),
			Kind: &tryjob.ExecutionLogEntry_TryjobsEnded_{
				TryjobsEnded: &tryjob.ExecutionLogEntry_TryjobsEnded{
					Tryjobs: endedTryjobLogs,
				},
			},
		})
	}

	switch hasFailed, hasNewAttemptToTrigger := len(failedIndices) > 0, len(p.triggerNewAttempt) > 0; {
	case !hasFailed && !hasNewAttemptToTrigger && !hasNonEndedCriticalTryjob:
		// All critical Tryjobs have completed successfully.
		execState.Status = tryjob.ExecutionState_SUCCEEDED
		return execState, nil, nil
	case hasFailed:
		if ok := e.canRetryAll(ctx, execState, failedIndices); !ok {
			execState.Status = tryjob.ExecutionState_FAILED
			if execState.Failures == nil {
				execState.Failures = &tryjob.ExecutionState_Failures{}
			}
			for _, tj := range failedTryjobs {
				execState.Failures.UnsuccessfulResults = append(execState.Failures.UnsuccessfulResults, &tryjob.ExecutionState_Failures_UnsuccessfulResult{
					TryjobId: int64(tj.ID),
				})
			}
			return execState, nil, nil
		}
		for _, idx := range failedIndices {
			p.triggerNewAttempt = append(p.triggerNewAttempt, planItem{
				definition: execState.GetRequirement().GetDefinitions()[idx],
				execution:  execState.GetExecutions()[idx],
			})
		}
	}
	return execState, &p, nil
}

// updateAttempts sets the attempts Status and Result from the corresponding
// Tryjobs.
//
// Return true if the latest attempt gets updated.
// Return false if the execution has no attempt.
func updateAttempts(exec *tryjob.ExecutionState_Execution, tryjobsByIDs map[common.TryjobID]*tryjob.Tryjob) bool {
	if len(exec.GetAttempts()) == 0 {
		return false
	}
	var latestAttemptUpdated bool
	latestAttemptIdx := len(exec.GetAttempts()) - 1
	for i, attempt := range exec.GetAttempts() {
		switch tj, ok := tryjobsByIDs[common.TryjobID(attempt.TryjobId)]; {
		case !ok:
			continue
		case i == latestAttemptIdx:
			if attempt.Status != tj.Status || !proto.Equal(attempt.Result, tj.Result) {
				attempt.Status = tj.Status
				attempt.Result = tj.Result
				latestAttemptUpdated = true
			}
		default: // blindly updates the attempt
			attempt.Status = tj.Status
			attempt.Result = tj.Result
		}
	}
	return latestAttemptUpdated

}

// hasExecutionStateEnded returns whether the status of ExecutionState is final.
func hasExecutionStateEnded(execState *tryjob.ExecutionState) bool {
	switch execState.GetStatus() {
	case tryjob.ExecutionState_FAILED, tryjob.ExecutionState_SUCCEEDED:
		return true
	default:
		return false
	}
}

// hasAttemptEnded whether the Attempt is in a final state.
func hasAttemptEnded(attempt *tryjob.ExecutionState_Execution_Attempt) bool {
	if attempt == nil {
		return false // Hasn't launched any new Attempt.
	}
	if attempt.Status == tryjob.Status_STATUS_UNSPECIFIED {
		panic(fmt.Errorf("attempt status not specified for Tryjob %d", attempt.TryjobId))
	}
	return tryjob.IsEnded(attempt.Status)
}

// handleRequirementChange updates `execState` and makes a plan when the
// Requirement changes.
//
// If some Definitions have been removed, some Tryjobs may be discarded, and if
// others have been added, then some new Tryjobs may need to be launched.
func handleRequirementChange(curReqmt, targetReqmt *tryjob.Requirement, execState *tryjob.ExecutionState) (*tryjob.ExecutionState, *plan, error) {
	existingExecutionByDef := make(map[*tryjob.Definition]*tryjob.ExecutionState_Execution, len(curReqmt.GetDefinitions()))
	for i, def := range curReqmt.GetDefinitions() {
		existingExecutionByDef[def] = execState.GetExecutions()[i]
	}
	reqmtDiff := requirement.Diff(curReqmt, targetReqmt)
	p := &plan{}
	// Populate discarded Tryjob.
	for def := range reqmtDiff.RemovedDefs {
		p.discard = append(p.discard, planItem{
			definition:    def,
			execution:     existingExecutionByDef[def],
			discardReason: noLongerRequiredInConfig,
		})
	}
	// Apply new Requirement and figure out which execution requires triggering
	// new Attempt.
	execState.Requirement = targetReqmt
	execState.Executions = execState.Executions[:0]
	for _, def := range targetReqmt.GetDefinitions() {
		switch {
		case reqmtDiff.AddedDefs.Has(def):
			exec := &tryjob.ExecutionState_Execution{}
			execState.Executions = append(execState.Executions, exec)
			p.triggerNewAttempt = append(p.triggerNewAttempt, planItem{
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
		case !reqmtDiff.UnchangedDefsReverse.Has(def):
			panic(fmt.Errorf("impossible; definition is not showing up in diff result. Definition: %s", def))
		default: // Nothing has changed for this definition.
			existingDef := reqmtDiff.UnchangedDefsReverse[def]
			exec, ok := existingExecutionByDef[existingDef]
			if !ok {
				panic(fmt.Errorf("impossible; definition shows up in diff but not in existing execution state. Definition: %s", existingDef))
			}
			execState.Executions = append(execState.Executions, exec)
		}
	}
	if len(targetReqmt.GetDefinitions()) == 0 {
		execState.Status = tryjob.ExecutionState_SUCCEEDED
	}
	return execState, p, nil
}

// executePlan executes the plan and mutates the ExecutionState.
//
// This includes starting Tryjobs and handling any failures to start Tryjobs.
func (e *Executor) executePlan(ctx context.Context, p *plan, r *run.Run, execState *tryjob.ExecutionState) (*tryjob.ExecutionState, error) {
	if len(p.discard) > 0 {
		// TODO(crbug/1323597): cancel the tryjobs because they are no longer
		// useful.
		for _, item := range p.discard {
			e.logTryjobDiscarded(ctx, item.definition, item.execution, item.discardReason)
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

		project, configGroup := r.ID.LUCIProject(), r.ConfigGroupID.Name()
		var earliestCreated time.Time
		for i, tj := range tryjobs {
			def, exec := definitions[i], executions[i]
			attempt := convertToAttempt(tj, r.ID)
			exec.Attempts = append(exec.Attempts, attempt)
			if !attempt.Reused && attempt.Status != tryjob.Status_UNTRIGGERED {
				isRetry := len(exec.Attempts) > 1
				e.stagedMetricReportFns = append(e.stagedMetricReportFns, func(ctx context.Context) {
					tryjob.RunWithBuilderMetricsTarget(ctx, e.Env, def, func(ctx context.Context) {
						metrics.Public.TryjobLaunched.Add(ctx, 1, project, configGroup, def.GetCritical(), isRetry)
					})
				})
				if tj.Result.GetCreateTime() != nil {
					if createTime := tj.Result.GetCreateTime().AsTime(); earliestCreated.IsZero() || createTime.Before(earliestCreated) {
						earliestCreated = createTime
					}
				}
			}
			if attempt.Status == tryjob.Status_UNTRIGGERED && def.GetCritical() {
				if execState.Failures == nil {
					execState.Failures = &tryjob.ExecutionState_Failures{}
				}
				execState.Failures.LaunchFailures = append(execState.Failures.LaunchFailures, &tryjob.ExecutionState_Failures_LaunchFailure{
					Definition: def,
					Reason:     tj.UntriggeredReason,
				})
			}
		}

		if !execState.FirstTryjobLatencyMetricsReported && !earliestCreated.IsZero() {
			e.stagedMetricReportFns = append(e.stagedMetricReportFns, func(ctx context.Context) {
				metricFields := []any{
					r.ID.LUCIProject(),
					r.ConfigGroupID.Name(),
					string(r.Mode),
				}
				metrics.Internal.CreateToFirstTryjobLatency.Add(ctx, float64(earliestCreated.Sub(r.CreateTime).Milliseconds()), metricFields...)
				metrics.Internal.StartToFirstTryjobLatency.Add(ctx, float64(earliestCreated.Sub(r.StartTime).Milliseconds()), metricFields...)
			})
			execState.FirstTryjobLatencyMetricsReported = true
		}

		if execState.Failures != nil {
			execState.Status = tryjob.ExecutionState_FAILED
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
