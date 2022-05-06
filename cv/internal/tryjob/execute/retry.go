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

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/tryjob"
)

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
) []planItem {
	var ret []planItem

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
			ret = append(ret, planItem{
				defintion: definition,
				execution: execution,
			})
		case execState.GetRequirement().GetRetryConfig() == nil:
			failedTryjobs = append(failedTryjobs, tryjobsByID[common.TryjobID(attempt.TryjobId)])
			logging.Debugf(ctx, "tryjobs cannot be retried because retries are not configured")
			execState.Status = tryjob.ExecutionState_FAILED
		default:
			// The tryjob is critical and failed, it would need to be retried for
			// the Run to succeed.
			ret = append(ret, planItem{
				defintion: definition,
				execution: execution,
			})
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
