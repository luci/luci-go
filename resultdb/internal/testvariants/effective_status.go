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

package testvariants

import (
	"fmt"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Represents the values of a virtual field used for sorting results by "effective"
// test verdict status V2 (that is, verdict status after override statuses like exoneration
// are applied).
type statusV2Effective int

const (
	// Defines the possible values of StatusV2Effective and their sort order.
	statusV2EffectiveFailed           statusV2Effective = 1
	statusV2EffectiveExecutionErrored statusV2Effective = 2
	statusV2EffectivePrecluded        statusV2Effective = 3
	statusV2EffectiveFlaky            statusV2Effective = 4
	statusV2EffectiveExonerated       statusV2Effective = 5

	// For performance reasons, passed and skipped results have equivalent sort priority.
	statusV2EffectivePassedOrSkipped statusV2Effective = 6
)

func (s statusV2Effective) String() string {
	switch s {
	case statusV2EffectiveExonerated:
		return "EXONERATED"
	case statusV2EffectivePassedOrSkipped:
		return "PASSED_OR_SKIPPED"
	case statusV2EffectiveFailed:
		return "FAILED"
	case statusV2EffectiveExecutionErrored:
		return "EXECUTION_ERRORED"
	case statusV2EffectivePrecluded:
		return "PRECLUDED"
	case statusV2EffectiveFlaky:
		return "FLAKY"
	default:
		panic("unknown StatusV2Effective")
	}
}

func parseStatusV2Effective(s string) (statusV2Effective, error) {
	switch s {
	case "EXONERATED":
		return statusV2EffectiveExonerated, nil
	case "PASSED_OR_SKIPPED":
		return statusV2EffectivePassedOrSkipped, nil
	case "FAILED":
		return statusV2EffectiveFailed, nil
	case "EXECUTION_ERRORED":
		return statusV2EffectiveExecutionErrored, nil
	case "PRECLUDED":
		return statusV2EffectivePrecluded, nil
	case "FLAKY":
		return statusV2EffectiveFlaky, nil
	default:
		return 0, fmt.Errorf("unknown StatusV2Effective: %s", s)
	}
}

func calculateStatusV2Effective(s pb.TestVerdict_Status, os pb.TestVerdict_StatusOverride) statusV2Effective {
	// This should be kept in sync with the SQL in query.go.
	switch {
	case os == pb.TestVerdict_EXONERATED:
		return statusV2EffectiveExonerated
	case s == pb.TestVerdict_PASSED || s == pb.TestVerdict_SKIPPED:
		return statusV2EffectivePassedOrSkipped
	case s == pb.TestVerdict_FAILED:
		return statusV2EffectiveFailed
	case s == pb.TestVerdict_EXECUTION_ERRORED:
		return statusV2EffectiveExecutionErrored
	case s == pb.TestVerdict_PRECLUDED:
		return statusV2EffectivePrecluded
	case s == pb.TestVerdict_FLAKY:
		return statusV2EffectiveFlaky
	default:
		panic(fmt.Sprintf("unknown test verdict status: %v", s))
	}
}
