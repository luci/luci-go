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
	"strings"

	"go.chromium.org/luci/common/logging"

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

// canRetry determines whether an individual attempt can be retried.
func canRetry(definition *tryjob.Definition, attempt *tryjob.ExecutionState_Execution_Attempt) retriability {
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

// canRetryAll checks whether all failed critical tryjobs can be retried.
//
// Returns true with empty string or false with the reason explaining why
// tryjobs can't be retried. Panics when any provided index points to a tryjob
// that is not critical or its latest attempt doesn't fail.
//
// Always updates the consumed quota for failed executions.
func canRetryAll(
	ctx context.Context,
	execState *tryjob.ExecutionState,
	failedIndices []int,
) (bool, string) {
	if execState.GetRequirement().GetRetryConfig() == nil {
		return false, "retry is not enabled for this Config Group"
	}

	var totalUsedQuota int32
	for _, execution := range execState.GetExecutions() {
		totalUsedQuota += execution.GetUsedQuota()
	}
	retryConfig := execState.GetRequirement().GetRetryConfig()
	var reasons []string
	for _, idx := range failedIndices {
		definition := execState.GetRequirement().GetDefinitions()[idx]
		exec := execState.GetExecutions()[idx]
		attempt := exec.GetAttempts()[len(exec.GetAttempts())-1]
		ensureTryjobCriticalAndFailed(definition, attempt)
		switch canRetry(definition, attempt) {
		case retryDenied:
			reasons = append(reasons, fmt.Sprintf("tryjob %s denies retry", attempt.GetExternalId()))
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
				reasons = append(reasons, fmt.Sprintf("tryjob %s cannot be retried due to insufficient quota", attempt.GetExternalId()))
			}
		default:
			panic(fmt.Errorf("unknown retriability"))
		}
	}

	if totalUsedQuota > retryConfig.GetGlobalQuota() {
		reasons = append(reasons, "tryjobs cannot be retried due to insufficient global quota")
	}
	switch len(reasons) {
	case 0:
		return true, ""
	case 1:
		return false, reasons[0]
	default:
		return false, fmt.Sprintf("can't retry because\n* %s", strings.Join(reasons, "\n* "))
	}
}
