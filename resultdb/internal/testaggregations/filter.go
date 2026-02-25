// Copyright 2025 The LUCI Authors.
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

package testaggregations

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ValidateVerdictStatusFilter validates a verdict effective status filter.
func ValidateVerdictStatusFilter(filter []pb.VerdictEffectiveStatus) error {
	seen := make(map[pb.VerdictEffectiveStatus]bool)
	for _, s := range filter {
		if s == pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_UNSPECIFIED {
			return errors.New("must not contain VERDICT_EFFECTIVE_STATUS_UNSPECIFIED")
		}
		if _, ok := pb.VerdictEffectiveStatus_name[int32(s)]; !ok {
			return errors.Fmt("contains unknown verdict effective status %v", s)
		}
		if seen[s] {
			return errors.Fmt("must not contain duplicates (%v appears twice)", s)
		}
		seen[s] = true
	}
	return nil
}

// ValidateModuleStatusFilter validates a module status filter.
func ValidateModuleStatusFilter(filter []pb.TestAggregation_ModuleStatus) error {
	seen := make(map[pb.TestAggregation_ModuleStatus]bool)
	for _, s := range filter {
		// MODULE_STATUS_UNSPECIFIED is allowed, because it is used on
		// the legacy module.
		if _, ok := pb.TestAggregation_ModuleStatus_name[int32(s)]; !ok {
			return errors.Fmt("contains unknown module status %v", s)
		}
		if seen[s] {
			return errors.Fmt("must not contain duplicates (%v appears twice)", s)
		}
		seen[s] = true
	}
	return nil
}

// whereVerdictStatusClause generates a WHERE clause for the given verdict status filter.
// This method assumes the presence of the columns IsExonerated and Status (containing the verdict base status).
func whereVerdictStatusClause(filter []pb.VerdictEffectiveStatus, params map[string]any) (string, error) {
	if len(filter) == 0 {
		// Empty filter means nothing matches.
		return "FALSE", nil
	}

	includeExonerated := false
	// The list of exonerable statuses to filter to (e.g. failed, flaky, ...).
	var exonerableStatuses []int64
	// The list of passed and skipped statuses to filter to.
	var passedAndSkippedStatuses []int64

	for _, s := range filter {
		switch s {
		case pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXONERATED:
			includeExonerated = true
		case pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FAILED:
			exonerableStatuses = append(exonerableStatuses, int64(pb.TestVerdict_FAILED))
		case pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_EXECUTION_ERRORED:
			exonerableStatuses = append(exonerableStatuses, int64(pb.TestVerdict_EXECUTION_ERRORED))
		case pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PRECLUDED:
			exonerableStatuses = append(exonerableStatuses, int64(pb.TestVerdict_PRECLUDED))
		case pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_FLAKY:
			exonerableStatuses = append(exonerableStatuses, int64(pb.TestVerdict_FLAKY))
		case pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_SKIPPED:
			passedAndSkippedStatuses = append(passedAndSkippedStatuses, int64(pb.TestVerdict_SKIPPED))
		case pb.VerdictEffectiveStatus_VERDICT_EFFECTIVE_STATUS_PASSED:
			passedAndSkippedStatuses = append(passedAndSkippedStatuses, int64(pb.TestVerdict_PASSED))
		default:
			return "", errors.Fmt("unsupported effective verdict status %s", s)
		}
	}

	var conditions []string
	if includeExonerated {
		// When filtering to exonerated verdicts, don't just check for an exoneration,
		// verify the status is exonerable.
		var allExonerableStatuses []int64
		allExonerableStatuses = append(allExonerableStatuses, int64(pb.TestVerdict_FAILED))
		allExonerableStatuses = append(allExonerableStatuses, int64(pb.TestVerdict_EXECUTION_ERRORED))
		allExonerableStatuses = append(allExonerableStatuses, int64(pb.TestVerdict_PRECLUDED))
		allExonerableStatuses = append(allExonerableStatuses, int64(pb.TestVerdict_FLAKY))

		paramName := "vsfAllExonerableStatuses"
		params[paramName] = allExonerableStatuses
		conditions = append(conditions, fmt.Sprintf("(IsExonerated AND VerdictStatus IN UNNEST(@%s))", paramName))
	}
	if len(passedAndSkippedStatuses) > 0 {
		// When filtering to passed or skipped verdicts, it doesn't matter if they have
		// an exoneration as it does not change the effective status.
		const paramName = "vsfPassedOrSkippedStatuses"
		params[paramName] = passedAndSkippedStatuses
		conditions = append(conditions, fmt.Sprintf("(VerdictStatus IN UNNEST(@%s))", paramName))
	}
	if len(exonerableStatuses) > 0 {
		// When filtering to failing, flaky, execution errored or precluded verdicts,
		// verify they have not been exonerated.
		const paramName = "vsfExonerableStatuses"
		params[paramName] = exonerableStatuses
		conditions = append(conditions, fmt.Sprintf("(NOT IsExonerated AND VerdictStatus IN UNNEST(@%s))", paramName))
	}

	if len(conditions) == 1 {
		return conditions[0], nil
	}
	return "(" + strings.Join(conditions, " OR ") + ")", nil
}

// whereModuleStatusFilter generates a WHERE clause for the given module status filter.
// This method assumes the presence of the column ModuleStatus.
func whereModuleStatusFilter(filter []pb.TestAggregation_ModuleStatus, params map[string]any) (string, error) {
	if len(filter) == 0 {
		// Empty filter means nothing matches.
		return "FALSE", nil
	}

	var statuses []int64
	for _, s := range filter {
		statuses = append(statuses, int64(s))
	}

	const paramName = "msfStatuses"
	params[paramName] = statuses
	return fmt.Sprintf("(ModuleStatus IN UNNEST(@%s))", paramName), nil
}
