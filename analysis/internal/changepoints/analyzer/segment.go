// Copyright 2024 The LUCI Authors.
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

package analyzer

import "time"

// Segment is a logical segment of a test variant branch.
// It represents the synthesis of segments identified in the input buffer
// and finalized/finalizing segments in the output buffer.
type Segment struct {
	// If set, means the segment commenced with a changepoint.
	// If unset, means the segment began with the beginning of recorded
	// history for the segment.
	HasStartChangepoint bool
	// The nominal commit position at which the segment starts (inclusive).
	StartPosition int64
	// The lower bound of the starting changepoint position in a 99% two-tailed
	// confidence interval. Inclusive.
	// Only set if HasStartChangepoint is set.
	StartPositionLowerBound99Th int64
	// The upper bound of the starting changepoint position in a 99% two-tailed
	// confidence interval. Inclusive.
	// Only set if HasStartChangepoint is set.
	StartPositionUpperBound99Th int64
	// The earliest hour a test run at the indicated nominal StartPosition
	// was recorded. Gives an approximate upper bound on the timestamp the
	// changepoint occurred, for systems which need to filter by date.
	StartHour time.Time
	// The nominal commit position at which the segment ends (inclusive).
	EndPosition int64
	// The earliest hour a test run at the indicated EndPosition
	// was recorded. Gives an approximate lower bound on the timestamp
	// the changepoint occurred, for systems which need to filter by date.
	EndHour time.Time
	// The most recent hour an unexpected test result was recorded in this segment.
	MostRecentUnexpectedResultHour time.Time
	// Total number of test results/runs/source verdicts in the segment.
	Counts Counts
}

// Counts contains statistics of the test results in a segment.
// It excludes all skipped test results.
type Counts struct {
	// The number of unexpected non-skipped test results.
	UnexpectedResults int64
	// The total number of non-skipped test results.
	TotalResults int64
	// The number of expected passed test results.
	ExpectedPassedResults int64
	// The number of expected failed test results.
	ExpectedFailedResults int64
	// The number of expected crashed test results.
	ExpectedCrashedResults int64
	// The number of expected aborted test results.
	ExpectedAbortedResults int64
	// The number of unexpected passed test results.
	UnexpectedPassedResults int64
	// The number of unexpected failed test results.
	UnexpectedFailedResults int64
	// The number of unexpected crashed test results.
	UnexpectedCrashedResults int64
	// The number of unexpected aborted test results.
	UnexpectedAbortedResults int64

	// The number of test runs which had an unexpected test result but were
	// not retried.
	UnexpectedUnretriedRuns int64
	// The number of test run which had an unexpected test result, were
	// retried, and still contained only unexpected test results.
	UnexpectedAfterRetryRuns int64
	// The number of test runs which had an unexpected test result, were
	// retried, and eventually recorded an expected test result.
	FlakyRuns int64
	// The total number of test runs.
	TotalRuns int64

	// The number of source verdicts which had only unexpected test results.
	UnexpectedSourceVerdicts int64
	// The number of source verdicts that had both unexpected and expected
	// test results.
	FlakySourceVerdicts int64
	// The total number of source verdicts.
	TotalSourceVerdicts int64
}
