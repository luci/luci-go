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
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/sources"
	"go.chromium.org/luci/analysis/internal/ingestion/resultdb"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// ToRuns converts a test verdict to a set of runs for the
// input buffer.
// The runs in verdict details are ordered by:
// - UnexpectedCount, descendingly, then
// - ExpectedCount, descendingly.
func ToRuns(tv *rdbpb.TestVariant, partitionTime time.Time, claimedInvs map[string]bool, src *pb.Sources) ([]inputbuffer.Run, error) {
	commitPosition := sources.CommitPosition(src)
	hour := partitionTime.Truncate(time.Hour)

	var result []inputbuffer.Run
	// runIndicies maps invocation name to result index.
	runIndicies := map[string]int{}

	for _, r := range tv.Results {
		tr := r.GetResult()
		invocationName, err := resultdb.InvocationFromTestResultName(tr.Name)
		if err != nil {
			return nil, errors.Annotate(err, "invocation from test result name").Err()
		}
		_, isClaimed := claimedInvs[invocationName]
		if !isClaimed {
			// Do not ingest duplicate runs. They cannot be used for changepoint analysis
			// as they distort counts of independent events in the segment statistics.
			continue
		}

		index, ok := runIndicies[invocationName]

		var run inputbuffer.Run
		if ok {
			run = result[index]
		} else {
			run = inputbuffer.Run{
				CommitPosition: commitPosition,
				Hour:           hour,
			}
			index = len(result)
			result = append(result, run)
			runIndicies[invocationName] = index
		}

		if tr.Expected {
			if tr.Status == rdbpb.TestStatus_PASS {
				run.Expected.PassCount++
			}
			if tr.Status == rdbpb.TestStatus_FAIL {
				run.Expected.FailCount++
			}
			if tr.Status == rdbpb.TestStatus_CRASH {
				run.Expected.CrashCount++
			}
			if tr.Status == rdbpb.TestStatus_ABORT {
				run.Expected.AbortCount++
			}
		} else {
			if tr.Status == rdbpb.TestStatus_PASS {
				run.Unexpected.PassCount++
			}
			if tr.Status == rdbpb.TestStatus_FAIL {
				run.Unexpected.FailCount++
			}
			if tr.Status == rdbpb.TestStatus_CRASH {
				run.Unexpected.CrashCount++
			}
			if tr.Status == rdbpb.TestStatus_ABORT {
				run.Unexpected.AbortCount++
			}
		}
		result[index] = run
	}

	return result, nil
}

func ToRun(v *rdbpb.RunTestVerdict, partitionTime time.Time, src *pb.Sources) inputbuffer.Run {
	commitPosition := sources.CommitPosition(src)
	hour := partitionTime.Truncate(time.Hour)

	result := inputbuffer.Run{
		CommitPosition: commitPosition,
		Hour:           hour,
	}
	for _, r := range v.Results {
		tr := r.GetResult()
		if tr.Expected {
			if tr.Status == rdbpb.TestStatus_PASS {
				result.Expected.PassCount++
			}
			if tr.Status == rdbpb.TestStatus_FAIL {
				result.Expected.FailCount++
			}
			if tr.Status == rdbpb.TestStatus_CRASH {
				result.Expected.CrashCount++
			}
			if tr.Status == rdbpb.TestStatus_ABORT {
				result.Expected.AbortCount++
			}
		} else {
			if tr.Status == rdbpb.TestStatus_PASS {
				result.Unexpected.PassCount++
			}
			if tr.Status == rdbpb.TestStatus_FAIL {
				result.Unexpected.FailCount++
			}
			if tr.Status == rdbpb.TestStatus_CRASH {
				result.Unexpected.CrashCount++
			}
			if tr.Status == rdbpb.TestStatus_ABORT {
				result.Unexpected.AbortCount++
			}
		}
	}
	return result
}
