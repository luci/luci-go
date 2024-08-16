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

	"go.chromium.org/luci/common/logging"
	"google.golang.org/protobuf/proto"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// retriability indicates whether a tryjob is allowed or denied to retry.
type retriability int

const (
	// The tryjob cannot be retried.
	retryDenied retriability = iota + 1
	// The tryjob needs to be retried if quota allows.
	retryAllowed
	// The tryjob failed, but was not triggered by this run, so trigger with
	// no regard for retry quota.
	retryAllowedIgnoreQuota
)

// canRetryAll checks whether all failed critical tryjobs can be retried.
//
// Panics when any provided index points to a tryjob that is not critical or
// its latest attempt doesn't fail.
//
// Always updates the consumed quota for failed executions.
func (e *Executor) canRetryAll(
	ctx context.Context,
	execState *tryjob.ExecutionState,
	failedIndices []int,
) bool {
	if !isRetryEnabled(execState.GetRequirement().GetRetryConfig()) {
		e.logRetryDenied(ctx, execState, failedIndices, "retry is not enabled in the config")
		return false
	}

	var totalUsedQuota int32
	canRetryAll := true
	for _, execution := range execState.GetExecutions() {
		totalUsedQuota += execution.GetUsedQuota()
	}
	retryConfig := execState.GetRequirement().GetRetryConfig()
	for _, idx := range failedIndices {
		definition := execState.GetRequirement().GetDefinitions()[idx]
		exec := execState.GetExecutions()[idx]
		attempt := tryjob.LatestAttempt(exec)
		ensureTryjobCriticalAndFailed(definition, attempt)
		switch canRetry(attempt) {
		case retryDenied:
			canRetryAll = false
			e.logRetryDenied(ctx, execState, []int{idx}, "tryjob explicitly denies retry in its output")
		case retryAllowedIgnoreQuota:
		case retryAllowed:
			var quotaToRetry int32
			switch attempt.Result.Status {
			case tryjob.Result_TIMEOUT:
				quotaToRetry = retryConfig.GetTimeoutWeight()
			case tryjob.Result_FAILED_PERMANENTLY:
				quotaToRetry = retryConfig.GetFailureWeight()
			case tryjob.Result_FAILED_TRANSIENTLY:
				quotaToRetry = retryConfig.GetTransientFailureWeight()
			default:
				logging.Errorf(ctx, "fallback to use failure status because of unexpected attempt result status %s", attempt.Result.Status)
				quotaToRetry = retryConfig.GetFailureWeight()
			}
			exec.UsedQuota += quotaToRetry
			totalUsedQuota += quotaToRetry
			if exec.UsedQuota > retryConfig.GetSingleQuota() {
				canRetryAll = false
				e.logRetryDenied(ctx, execState, []int{idx}, "insufficient quota")
			}
		default:
			panic(fmt.Errorf("unknown retriability"))
		}
	}

	if canRetryAll && totalUsedQuota > retryConfig.GetGlobalQuota() {
		canRetryAll = false
		e.logRetryDenied(ctx, execState, failedIndices, "insufficient global quota")
	}
	return canRetryAll
}

func ensureTryjobCriticalAndFailed(def *tryjob.Definition, attempt *tryjob.ExecutionState_Execution_Attempt) {
	switch {
	case !def.Critical:
		panic(fmt.Errorf("calling canRetryAll for non critical tryjob: %s", def))
	case attempt.Status != tryjob.Status_ENDED:
		panic(fmt.Errorf("calling canRetryAll for non ended tryjob: %s", def))
	case attempt.Result.Status == tryjob.Result_SUCCEEDED:
		panic(fmt.Errorf("calling canRetryAll for succeeded tryjob: %s", def))
	}
}

// canRetry determines whether an individual attempt can be retried.
func canRetry(attempt *tryjob.ExecutionState_Execution_Attempt) retriability {
	switch {
	// If this run did not trigger the tryjob, do not count its failure
	// towards retry quota. Also ignores the output_retry_denied property.
	case attempt.Reused:
		return retryAllowedIgnoreQuota
	// Check if the tryjob forbids retry.
	case attempt.Result.GetOutput().GetRetry() == recipe.Output_OUTPUT_RETRY_DENIED:
		return retryDenied
	default:
		return retryAllowed
	}
}

var emptyRetryConfig = &cfgpb.Verifiers_Tryjob_RetryConfig{}

func isRetryEnabled(rc *cfgpb.Verifiers_Tryjob_RetryConfig) bool {
	switch {
	case rc == nil:
		return false
	case proto.Equal(rc, emptyRetryConfig):
		return false
	default:
		return true
	}
}
