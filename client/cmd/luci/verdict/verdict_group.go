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

package verdict

import (
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type VerdictKey struct {
	TestID      string
	VariantHash string
}

type VerdictGroup struct {
	Key          VerdictKey
	Variant      *pb.Variant
	Results      []*pb.TestResult
	Exonerations []*pb.TestExoneration
}

func (g *VerdictGroup) Status() string {
	hasPassed := false
	hasFailed := false
	hasSkipped := false
	hasExecutionErrored := false
	hasPrecluded := false

	for _, r := range g.Results {
		switch r.StatusV2 {
		case pb.TestResult_PASSED:
			hasPassed = true
		case pb.TestResult_FAILED:
			hasFailed = true
		case pb.TestResult_SKIPPED:
			hasSkipped = true
		case pb.TestResult_EXECUTION_ERRORED:
			hasExecutionErrored = true
		case pb.TestResult_PRECLUDED:
			hasPrecluded = true
		}
	}

	if hasPassed && hasFailed {
		return "FLAKY"
	}
	if hasFailed {
		return "FAILED"
	}
	if hasExecutionErrored {
		return "EXECUTION_ERRORED"
	}
	if hasPrecluded {
		return "PRECLUDED"
	}
	if hasPassed {
		return "PASSED"
	}
	if hasSkipped {
		return "SKIPPED"
	}
	return "STATUS_UNSPECIFIED"
}
