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

	"go.chromium.org/luci/cv/api/recipe/v1"
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
func Do(ctx context.Context, r *run.Run, t *tryjob.ExecuteTryjobsPayload, shouldStop func() bool) (*tryjob.ExecuteTryjobsResult, error) {
	execState, stateVer, err := loadExecutionState(ctx, r.ID)
	switch {
	case err != nil:
		return nil, err
	case execState == nil:
		execState = &tryjob.ExecutionState{}
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

type plan struct {
	// new tryjob to trigger due to new builder in requirement.
	trigger []*tryjob.Definition

	// retry the existing tryjob due to transient failure.
	retry map[*tryjob.Definition]*tryjob.ExecutionState_Execution

	// discard the tryjob due to the builder is removed in the Project Config.
	discarded map[*tryjob.Definition]*tryjob.ExecutionState_Execution
}

func prepExecutionPlan(ctx context.Context, execState *tryjob.ExecutionState, r *run.Run, tryjobsUpdated []int64, reqmtChanged bool) (*plan, error) {
	switch {
	case len(tryjobsUpdated) > 0 && reqmtChanged:
		panic(fmt.Errorf("the executor can't handle requirement update and tryjobs update at the same time"))
	case len(tryjobsUpdated) > 0:
		// TODO(robertocn): update the state and compute the plan
		tryjobsByID, err := tryjob.LoadTryjobsMapByIDs(ctx, common.MakeTryjobIDs(tryjobsUpdated...))
		if err != nil {
			return nil, err
		}
		updateAttempts(execState, tryjobsByID)

		return &plan{retry: computeRetries(ctx, execState, tryjobsByID)}, nil
	case reqmtChanged:
		// TODO(yiwzhang): compute the plan
		return &plan{}, nil
	default:
		panic(fmt.Errorf("the executor is called without any update on either tryjobs or requirement"))
	}
}

// execute executes the plan and mutate the state.
func (p plan) execute(ctx context.Context, r *run.Run, execState *tryjob.ExecutionState) (*tryjob.ExecuteTryjobsResult, error) {
	panic("implement")
}

// updateAttempts updates the attempts associated with the given tryjobs.
func updateAttempts(execState *tryjob.ExecutionState, tryjobsToUpdateByID map[common.TryjobID]*tryjob.Tryjob) {
	for _, exec := range execState.Executions {
		if len(exec.Attempts) == 0 {
			continue
		}
		// Only look at the most recent attempt in the execution.
		lastAttempt := exec.Attempts[len(exec.Attempts)-1]
		if tj, present := tryjobsToUpdateByID[common.TryjobID(lastAttempt.TryjobId)]; present {
			lastAttempt.Result = tj.Result
			lastAttempt.Status = tj.Status
		}
	}
}

// retriability indicates whether a tryjob can/needs to be retried.
type retriability int

const (
	// There is no need to retry this tryjob, it has not failed, or is not
	// critical.
	retryNotNeeded retriability = iota
	// The tryjob cannot be retried, implies failure.
	retryDenied
	// The tryjob needs to be retried if quota allows.
	retryRequired
	// The tryjob failed, but was not triggered by this run, so trigger with
	// no regard for retry quota.
	retryRequiredIgnoreQuota
)

// needsRetry determines if a given execution's most recent attempt needs to
// be retried, and whether this is allowed, not accounting for retry quota.
func needsRetry(ctx context.Context, definition *tryjob.Definition, execution *tryjob.ExecutionState_Execution) retriability {
	if len(execution.Attempts) == 0 {
		panic("Execution has no attempts")
	}
	switch lastAttempt := execution.Attempts[len(execution.Attempts)-1]; {
	// Do not retry non-critical tryjobs regardless of result.
	case !definition.Critical:
		return retryNotNeeded
	case lastAttempt.Status != tryjob.Status_ENDED || lastAttempt.Result.Status == tryjob.Result_SUCCEEDED:
		return retryNotNeeded
	// If this run did not trigger the tryjob, do not count its failure
	// towards retry quota. Also ignores the output_retry_denied property.
	case lastAttempt.Reused:
		return retryRequiredIgnoreQuota
	// Check if the tryjob forbids retry.
	case lastAttempt.Result.GetOutput().GetRetry() == recipe.Output_OUTPUT_RETRY_DENIED:
		logging.Debugf(ctx, "tryjob %d cannot be retried as per recipe output", lastAttempt.TryjobId)
		return retryDenied
	default:
		return retryRequired
	}
}

// computeRetries gets the tryjobs to retry if quotas allow.
//
// It updates the execState status if the tryjobs cannot be retried.
func computeRetries(
	ctx context.Context,
	execState *tryjob.ExecutionState,
	tryjobsByID map[common.TryjobID]*tryjob.Tryjob,
) map[*tryjob.Definition]*tryjob.ExecutionState_Execution {
	ret := make(map[*tryjob.Definition]*tryjob.ExecutionState_Execution)

	var totalUsedQuota int32
	var failedTryjobs []*tryjob.Tryjob
	for i, execution := range execState.Executions {
		definition := execState.GetRequirement().GetDefinitions()[i]
		r := needsRetry(ctx, definition, execution)
		attempt := execution.Attempts[len(execution.Attempts)-1]
		totalUsedQuota += execution.UsedQuota
		switch {
		case r == retryNotNeeded:
		case r == retryDenied:
			failedTryjobs = append(failedTryjobs, tryjobsByID[common.TryjobID(attempt.TryjobId)])
			execState.Status = tryjob.ExecutionState_FAILED
		case r == retryRequiredIgnoreQuota:
			ret[definition] = execution
		case execState.GetRequirement().GetRetryConfig() == nil:
			failedTryjobs = append(failedTryjobs, tryjobsByID[common.TryjobID(attempt.TryjobId)])
			logging.Debugf(ctx, "tryjobs cannot be retried because retries are not configured")
			execState.Status = tryjob.ExecutionState_FAILED
		default:
			// The tryjob is critical and failed, it would need to be retried for
			// the Run to succeed.
			ret[definition] = execution
			failedTryjobs = append(failedTryjobs, tryjobsByID[common.TryjobID(attempt.TryjobId)])
			var quotaToRetry int32
			switch attempt.Result.Status {
			case tryjob.Result_TIMEOUT:
				quotaToRetry = execState.GetRequirement().GetRetryConfig().GetTimeoutWeight()
			case tryjob.Result_FAILED_PERMANENTLY:
				quotaToRetry = execState.GetRequirement().GetRetryConfig().GetFailureWeight()
			case tryjob.Result_FAILED_TRANSIENTLY:
				quotaToRetry = execState.GetRequirement().GetRetryConfig().GetTransientFailureWeight()
			default:
				logging.Errorf(ctx, "unexpected status %s", attempt.Result.Status)
				quotaToRetry = execState.GetRequirement().GetRetryConfig().GetFailureWeight()
			}
			execution.UsedQuota += quotaToRetry
			totalUsedQuota += quotaToRetry
			if execution.UsedQuota > execState.Requirement.RetryConfig.GetSingleQuota() {
				logging.Debugf(ctx, "tryjob %d cannot be retried due to insufficient quota", attempt.TryjobId)
				execState.Status = tryjob.ExecutionState_FAILED
			}
		}
	}
	if totalUsedQuota > execState.GetRequirement().GetRetryConfig().GetGlobalQuota() {
		logging.Debugf(ctx, "tryjobs cannot be retried due to insufficient global quota")
		execState.Status = tryjob.ExecutionState_FAILED
	}
	if execState.Status == tryjob.ExecutionState_FAILED {
		execState.FailureReason = composeReason(failedTryjobs)
		return nil
	}
	return ret
}
