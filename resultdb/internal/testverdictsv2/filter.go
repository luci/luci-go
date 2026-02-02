// Copyright 2026 The LUCI Authors.
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

package testverdictsv2

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ValidateFilter validates a verdict effective status filter.
func ValidateFilter(filter []pb.TestVerdictPredicate_VerdictEffectiveStatus) error {
	seen := make(map[pb.TestVerdictPredicate_VerdictEffectiveStatus]bool)
	for _, s := range filter {
		if s == pb.TestVerdictPredicate_VERDICT_EFFECTIVE_STATUS_UNSPECIFIED {
			return errors.New("must not contain VERDICT_EFFECTIVE_STATUS_UNSPECIFIED")
		}
		if _, ok := pb.TestVerdictPredicate_VerdictEffectiveStatus_name[int32(s)]; !ok {
			return errors.Fmt("contains unknown verdict effective status %v", s)
		}
		if seen[s] {
			return errors.Fmt("must not contain duplicates (%v appears twice)", s)
		}
		seen[s] = true
	}
	return nil
}

// whereClause generates a WHERE clause for the given filter.
// This method assumes the presence of the columns Status and StatusOverride.
func whereClause(filter []pb.TestVerdictPredicate_VerdictEffectiveStatus, params map[string]any) (string, error) {
	if len(filter) == 0 {
		// Empty filter means no filter.
		return "TRUE", nil
	}

	includeExonerated := false
	var statuses []int64

	for _, s := range filter {
		switch s {
		case pb.TestVerdictPredicate_EXONERATED:
			includeExonerated = true
		case pb.TestVerdictPredicate_FAILED:
			statuses = append(statuses, int64(pb.TestVerdict_FAILED))
		case pb.TestVerdictPredicate_EXECUTION_ERRORED:
			statuses = append(statuses, int64(pb.TestVerdict_EXECUTION_ERRORED))
		case pb.TestVerdictPredicate_PRECLUDED:
			statuses = append(statuses, int64(pb.TestVerdict_PRECLUDED))
		case pb.TestVerdictPredicate_FLAKY:
			statuses = append(statuses, int64(pb.TestVerdict_FLAKY))
		case pb.TestVerdictPredicate_SKIPPED:
			statuses = append(statuses, int64(pb.TestVerdict_SKIPPED))
		case pb.TestVerdictPredicate_PASSED:
			statuses = append(statuses, int64(pb.TestVerdict_PASSED))
		default:
			return "", errors.Fmt("unsupported effective verdict status %s", s)
		}
	}

	var conditions []string
	if includeExonerated {
		conditions = append(conditions, fmt.Sprintf("(StatusOverride = %d)", int64(pb.TestVerdict_EXONERATED)))
	}
	if len(statuses) > 0 {
		paramName := "effectiveStatusFilterStatuses"
		params[paramName] = statuses
		conditions = append(conditions, fmt.Sprintf("(StatusOverride = %d AND Status IN UNNEST(@%s))", int64(pb.TestVerdict_NOT_OVERRIDDEN), paramName))
	}

	if len(conditions) == 1 {
		return conditions[0], nil
	}
	return "(" + strings.Join(conditions, " OR ") + ")", nil
}

// hasOnlyPriorityVerdicts returns true if the given filter contains only non-priority verdicts
// (failed, flaky, execution errored, precluded or exonerated).
func hasOnlyPriorityVerdicts(filter []pb.TestVerdictPredicate_VerdictEffectiveStatus) bool {
	if len(filter) == 0 {
		// Empty filter means no filter.
		return false
	}

	for _, s := range filter {
		if s == pb.TestVerdictPredicate_PASSED || s == pb.TestVerdictPredicate_SKIPPED {
			return false
		}
		// Otherwise, the underlying status will be one of:
		// - FAILED
		// - FLAKY
		// - EXECUTION_ERRORED
		// - PRECLUDED
		// These require one of the test results to be failed, execution errored or precluded.
	}
	return true
}

// hasOnlyNonPriorityVerdicts returns true if the given filter contains only non-priority verdicts
// (passed or skipped).
func hasOnlyNonPriorityVerdicts(filter []pb.TestVerdictPredicate_VerdictEffectiveStatus) bool {
	if len(filter) == 0 {
		// Empty filter means no filter.
		return false
	}
	for _, s := range filter {
		if s != pb.TestVerdictPredicate_PASSED && s != pb.TestVerdictPredicate_SKIPPED {
			return false
		}
	}
	return true
}
