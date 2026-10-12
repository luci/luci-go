// Copyright 2023 The LUCI Authors.
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

package testvariantbranch

import (
	"time"

	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// resultStatusV1 returns the v1 TestStatus and Expected flag for a TestResult,
// deriving them from StatusV2 if the legacy Status field is unspecified.
func resultStatusV1(tr *rdbpb.TestResult) (rdbpb.TestStatus, bool) {
	if tr.Status == rdbpb.TestStatus_STATUS_UNSPECIFIED && tr.StatusV2 != rdbpb.TestResult_STATUS_UNSPECIFIED {
		return rdbpbutil.TestStatusV1FromV2(tr.StatusV2, tr.FailureReason.GetKind(), tr.FrameworkExtensions.GetWebTest())
	}
	return tr.Status, tr.Expected
}

// ToRuns converts a test verdict to a set of runs for the
// input buffer.
// The runs in verdict details are ordered by:
// - UnexpectedCount, descendingly, then
// - ExpectedCount, descendingly.
func ToRuns(tv *rdbpb.TestVariant, partitionTime time.Time, claimedInvs map[string]bool, src *pb.Sources) ([]inputbuffer.Run, error) {
	commitPosition := pbutil.SourcePosition(src)
	hour := partitionTime.Truncate(time.Hour)

	var result []inputbuffer.Run
	// runIndicies maps invocation name to result index.
	runIndicies := map[string]int{}

	for _, r := range tv.Results {
		tr := r.GetResult()
		invocationName, err := resultdb.InvocationFromTestResultName(tr.Name)
		if err != nil {
			return nil, errors.Fmt("invocation from test result name: %w", err)
		}
		_, isClaimed := claimedInvs[invocationName]
		if !isClaimed {
			// Do not ingest duplicate runs. They cannot be used for changepoint analysis
			// as they distort counts of independent events in the segment statistics.
			continue
		}
		status, expected := resultStatusV1(tr)
		if status == rdbpb.TestStatus_SKIP {
			// Do not ingest skipped (or execution errored / precluded) results.
			continue
		}

		index, ok := runIndicies[invocationName]

		var run inputbuffer.Run
		if ok {
			run = result[index]
		} else {
			run = inputbuffer.Run{
				SourcePosition: commitPosition,
				Hour:           hour,
			}
			index = len(result)
			result = append(result, run)
			runIndicies[invocationName] = index
		}

		if expected {
			if status == rdbpb.TestStatus_PASS {
				run.Expected.PassCount++
			}
			if status == rdbpb.TestStatus_FAIL {
				run.Expected.FailCount++
			}
			if status == rdbpb.TestStatus_CRASH {
				run.Expected.CrashCount++
			}
			if status == rdbpb.TestStatus_ABORT {
				run.Expected.AbortCount++
			}
		} else {
			if status == rdbpb.TestStatus_PASS {
				run.Unexpected.PassCount++
			}
			if status == rdbpb.TestStatus_FAIL {
				run.Unexpected.FailCount++
			}
			if status == rdbpb.TestStatus_CRASH {
				run.Unexpected.CrashCount++
			}
			if status == rdbpb.TestStatus_ABORT {
				run.Unexpected.AbortCount++
			}
		}
		result[index] = run
	}

	return result, nil
}

func ToRun(v *rdbpb.RunTestVerdict, partitionTime time.Time, src *pb.Sources) inputbuffer.Run {
	commitPosition := pbutil.SourcePosition(src)
	hour := partitionTime.Truncate(time.Hour)

	result := inputbuffer.Run{
		SourcePosition: commitPosition,
		Hour:           hour,
	}
	for _, r := range v.Results {
		tr := r.GetResult()
		status, expected := resultStatusV1(tr)
		if expected {
			if status == rdbpb.TestStatus_PASS {
				result.Expected.PassCount++
			}
			if status == rdbpb.TestStatus_FAIL {
				result.Expected.FailCount++
			}
			if status == rdbpb.TestStatus_CRASH {
				result.Expected.CrashCount++
			}
			if status == rdbpb.TestStatus_ABORT {
				result.Expected.AbortCount++
			}
		} else {
			if status == rdbpb.TestStatus_PASS {
				result.Unexpected.PassCount++
			}
			if status == rdbpb.TestStatus_FAIL {
				result.Unexpected.FailCount++
			}
			if status == rdbpb.TestStatus_CRASH {
				result.Unexpected.CrashCount++
			}
			if status == rdbpb.TestStatus_ABORT {
				result.Unexpected.AbortCount++
			}
		}
	}
	return result
}
