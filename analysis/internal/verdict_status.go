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

package internal

// Status of a Verdict.
// It is determined by all the test results of the verdict, and exonerations are
// ignored(i.e. failure is treated as a failure, even if it is exonerated).
type VerdictStatus int32

const (
	// A verdict must not have this status.
	// This is only used when filtering verdicts.
	VerdictStatus_VERDICT_STATUS_UNSPECIFIED VerdictStatus = 0
	// All results of the verdict are unexpected.
	VerdictStatus_UNEXPECTED VerdictStatus = 10
	// The verdict has both expected and unexpected results.
	// To be differentiated with AnalyzedTestVariantStatus.FLAKY.
	VerdictStatus_VERDICT_FLAKY VerdictStatus = 30
	// All results of the verdict are expected.
	VerdictStatus_EXPECTED VerdictStatus = 50
)

func (x VerdictStatus) String() string {
	switch x {
	case VerdictStatus_VERDICT_STATUS_UNSPECIFIED:
		return "VERDICT_STATUS_UNSPECIFIED"
	case VerdictStatus_UNEXPECTED:
		return "UNEXPECTED"
	case VerdictStatus_VERDICT_FLAKY:
		return "VERDICT_FLAKY"
	case VerdictStatus_EXPECTED:
		return "EXPECTED"
	default:
		return "UNKNOWN"
	}
}
