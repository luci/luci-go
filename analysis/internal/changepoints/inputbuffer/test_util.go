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

package inputbuffer

import (
	"time"
)

func VerdictRefs(positions, total, hasUnexpected []int) []*Run {
	return copyAndUnflattenRuns(Verdicts(positions, total, hasUnexpected))
}

func Verdicts(positions, total, hasUnexpected []int) []Run {
	retried := make([]int, len(total))
	unexpectedAfterRetry := make([]int, len(total))
	return VerdictsWithRetries(positions, total, hasUnexpected, retried, unexpectedAfterRetry)
}

func VerdictsWithRetriesRefs(positions, total, hasUnexpected, retried, unexpectedAfterRetry []int) []*Run {
	return copyAndUnflattenRuns(VerdictsWithRetries(positions, total, hasUnexpected, retried, unexpectedAfterRetry))
}

func VerdictsWithRetries(positions, total, hasUnexpected, retried, unexpectedAfterRetry []int) []Run {
	if len(total) != len(hasUnexpected) {
		panic("length mismatch between total and hasUnexpected")
	}
	if len(total) != len(retried) {
		panic("length mismatch between total and retried")
	}
	if len(total) != len(unexpectedAfterRetry) {
		panic("length mismatch between total and unexpectedAfterRetry")
	}
	var result []Run
	for i := range total {
		// From top to bottom, these are increasingly restrictive.
		totalCount := total[i]                               // Total number of test runs in this verdict.
		hasUnexpectedCount := hasUnexpected[i]               // How many of those test runs had at least one unexpected result.
		retriedCount := retried[i]                           // As above, plus at least two results in total.
		unexpectedAfterRetryCount := unexpectedAfterRetry[i] // As above, plus all test runs have only unexpected results.

		baseRun := Run{
			CommitPosition: int64(positions[i]),
			Hour:           time.Unix(int64(3600*(positions[i])), 0),
		}
		for i := range totalCount {
			run := baseRun
			if i < unexpectedAfterRetryCount {
				run.Unexpected = ResultCounts{
					FailCount:  1,
					CrashCount: 1,
				}
			} else if i < retriedCount {
				run.Expected = ResultCounts{
					PassCount: 1,
				}
				run.Unexpected = ResultCounts{
					FailCount: 1,
				}
			} else if i < hasUnexpectedCount {
				run.Unexpected = ResultCounts{
					FailCount: 1,
				}
			} else {
				run.Expected = ResultCounts{
					PassCount: 1,
				}
			}
			result = append(result, run)
		}
	}
	return result
}
