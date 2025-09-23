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

package stability

import (
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestQueryStability(t *testing.T) {
	ftt.Run("QueryStability", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		var1 := pbutil.Variant("key1", "val1", "key2", "val1")
		var3 := pbutil.Variant("key1", "val2", "key2", "val2")

		err := CreateQueryStabilityTestData(ctx)
		assert.Loosely(t, err, should.BeNil)

		opts := QueryStabilitySampleRequest()
		expectedResult := QueryStabilitySampleResponse()
		txn, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		t.Run("Baseline", func(t *ftt.Test) {
			result, err := QueryStability(txn, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(expectedResult))
		})
		t.Run("Flake analysis uses full 14 days if MinWindow unmet", func(t *ftt.Test) {
			opts.Criteria.FlakeRate.MinWindow = 100
			result, err := QueryStability(txn, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(QueryStabilitySampleResponseLargeWindow()))
		})
		t.Run("Project filter works correctly", func(t *ftt.Test) {
			opts.Project = "none"
			expectedResult = []*pb.TestVariantStabilityAnalysis{
				emptyStabilityAnalysis("test_id", var1),
				emptyStabilityAnalysis("test_id", var3),
			}

			result, err := QueryStability(txn, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(expectedResult))
		})
		t.Run("Realm filter works correctly", func(t *ftt.Test) {
			// No data exists in this realm.
			opts.SubRealms = []string{"otherrealm"}
			expectedResult = []*pb.TestVariantStabilityAnalysis{
				emptyStabilityAnalysis("test_id", var1),
				emptyStabilityAnalysis("test_id", var3),
			}

			result, err := QueryStability(txn, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(expectedResult))
		})
		t.Run("Works for tests without data", func(t *ftt.Test) {
			notExistsVariant := pbutil.Variant("key1", "val1", "key2", "not_exists")
			opts.TestVariantPositions = append(opts.TestVariantPositions,
				&pb.QueryTestVariantStabilityRequest_TestVariantPosition{
					TestId:  "not_exists_test_id",
					Variant: var1,
					Sources: &pb.Sources{
						BaseSources: &pb.Sources_GitilesCommit{
							GitilesCommit: &pb.GitilesCommit{
								Host:       "mysources.googlesource.com",
								Project:    "myproject/src",
								Ref:        "refs/heads/mybranch",
								CommitHash: "aabbccddeeff00112233aabbccddeeff00112233",
								Position:   130,
							},
						},
					},
				},
				&pb.QueryTestVariantStabilityRequest_TestVariantPosition{
					TestId:  "test_id",
					Variant: notExistsVariant,
					Sources: &pb.Sources{
						BaseSources: &pb.Sources_GitilesCommit{
							GitilesCommit: &pb.GitilesCommit{
								Host:       "mysources.googlesource.com",
								Project:    "myproject/src",
								Ref:        "refs/heads/mybranch",
								CommitHash: "aabbccddeeff00112233aabbccddeeff00112233",
								Position:   130,
							},
						},
					},
				})

			expectedResult = append(expectedResult,
				emptyStabilityAnalysis("not_exists_test_id", var1),
				emptyStabilityAnalysis("test_id", notExistsVariant))

			result, err := QueryStability(txn, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(expectedResult))
		})
		t.Run("Batching works correctly", func(t *ftt.Test) {
			// Ensure the order of test variants in the request and response
			// remain correct even when there are multiple batches.
			var expandedInput []*pb.QueryTestVariantStabilityRequest_TestVariantPosition
			var expectedOutput []*pb.TestVariantStabilityAnalysis
			for i := range batchSize {
				testID := fmt.Sprintf("test_id_%v", i)
				expandedInput = append(expandedInput, &pb.QueryTestVariantStabilityRequest_TestVariantPosition{
					TestId:  testID,
					Variant: var1,
					Sources: &pb.Sources{
						BaseSources: &pb.Sources_GitilesCommit{
							GitilesCommit: &pb.GitilesCommit{
								Host:       "mysources.googlesource.com",
								Project:    "myproject/src",
								Ref:        "refs/heads/mybranch",
								CommitHash: "aabbccddeeff00112233aabbccddeeff00112233",
								Position:   130,
							},
						},
					},
				})
				expectedOutput = append(expectedOutput, emptyStabilityAnalysis(testID, var1))
			}

			opts.TestVariantPositions = append(expandedInput, opts.TestVariantPositions...)
			expectedResult = append(expectedOutput, expectedResult...)

			result, err := QueryStability(txn, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble(expectedResult))
		})
	})
}

// emptyStabilityAnalysis returns an empty stability analysis proto.
func emptyStabilityAnalysis(testID string, variant *pb.Variant) *pb.TestVariantStabilityAnalysis {
	return &pb.TestVariantStabilityAnalysis{
		TestId:      testID,
		Variant:     variant,
		FailureRate: &pb.TestVariantStabilityAnalysis_FailureRate{},
		FlakeRate:   &pb.TestVariantStabilityAnalysis_FlakeRate{},
	}
}

func TestQueryStabilityHelpers(t *testing.T) {
	ftt.Run("flattenSourceVerdictsToRuns", t, func(t *ftt.Test) {
		unexpectedRun := run{expected: false}
		expectedRun := run{expected: true}
		t.Run("With no verdicts", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{}
			result := flattenSourceVerdictsToRuns(verdicts)
			assert.Loosely(t, result, should.HaveLength(0))
		})
		t.Run("With one unexpected run", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					UnexpectedRuns: 1,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			assert.Loosely(t, result, should.Resemble([]run{unexpectedRun}))
		})
		t.Run("With many unexpected runs", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					UnexpectedRuns: 3,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			assert.Loosely(t, result, should.Resemble([]run{unexpectedRun, unexpectedRun, unexpectedRun}))
		})
		t.Run("With one expected run", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns: 1,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			assert.Loosely(t, result, should.Resemble([]run{expectedRun}))
		})
		t.Run("With many expected run", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns: 3,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			assert.Loosely(t, result, should.Resemble([]run{expectedRun, expectedRun, expectedRun}))
		})
		t.Run("With mixed runs, evenly split", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns:   3,
					UnexpectedRuns: 3,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			assert.Loosely(t, result, should.Resemble([]run{unexpectedRun, expectedRun, unexpectedRun, expectedRun, unexpectedRun, expectedRun}))
		})
		t.Run("With mixed runs, 3/2 split", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns:   4,
					UnexpectedRuns: 2,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			assert.Loosely(t, result, should.Resemble([]run{unexpectedRun, expectedRun, expectedRun, unexpectedRun, expectedRun, expectedRun}))
		})
		t.Run("With multiple verdicts", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns: 2,
				},
				{
					ExpectedRuns:   1,
					UnexpectedRuns: 1,
				},
				{
					UnexpectedRuns: 2,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			assert.Loosely(t, result, should.Resemble([]run{expectedRun, expectedRun, unexpectedRun, expectedRun, unexpectedRun, unexpectedRun}))
		})
	})
	ftt.Run("truncateSourceVerdicts", t, func(t *ftt.Test) {
		t.Run("With no verdicts", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{}
			result := truncateSourceVerdicts(verdicts, 10)
			assert.Loosely(t, result, should.HaveLength(0))
		})
		t.Run("With large expected verdict", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns: 11,
				},
			}
			result := truncateSourceVerdicts(verdicts, 10)
			assert.Loosely(t, result, should.Resemble([]*sourceVerdict{
				{
					ExpectedRuns: 10,
				},
			}))
		})
		t.Run("With large unexpected verdict", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					UnexpectedRuns: 111,
				},
			}
			result := truncateSourceVerdicts(verdicts, 10)
			assert.Loosely(t, result, should.Resemble([]*sourceVerdict{
				{
					UnexpectedRuns: 10,
				},
			}))
		})
		t.Run("With multiple verdicts", func(t *ftt.Test) {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns: 2,
				},
				{
					ExpectedRuns:   8,
					UnexpectedRuns: 8,
				},
				{
					UnexpectedRuns: 3,
				},
			}
			result := truncateSourceVerdicts(verdicts, 10)
			assert.Loosely(t, result, should.Resemble([]*sourceVerdict{
				{
					ExpectedRuns: 2,
				},
				{
					ExpectedRuns:   4,
					UnexpectedRuns: 4,
				},
			}))
		})
	})
	ftt.Run("consecutiveFailureCount", t, func(t *ftt.Test) {
		t.Run("Consecutive from start and/or end", func(t *ftt.Test) {
			type testCase struct {
				runs     []run
				expected int
			}

			// Assume 10 runs, split 4 after / 2 on / 4 before.
			testCases := []testCase{
				{
					runs:     expectedRuns(10),
					expected: 0,
				},
				{
					runs:     combine(unexpectedRuns(1), expectedRuns(9)),
					expected: 0,
				},
				{
					runs:     combine(unexpectedRuns(2), expectedRuns(8)),
					expected: 0,
				},
				{
					runs:     combine(unexpectedRuns(3), expectedRuns(7)),
					expected: 0,
				},
				{
					runs:     combine(unexpectedRuns(4), expectedRuns(6)),
					expected: 0,
				},
				{
					runs:     combine(unexpectedRuns(5), expectedRuns(5)),
					expected: 0,
				},
				{
					runs:     combine(unexpectedRuns(6), expectedRuns(4)),
					expected: 6,
				},
				{
					runs:     combine(unexpectedRuns(7), expectedRuns(3)),
					expected: 7,
				},
				{
					runs:     combine(unexpectedRuns(8), expectedRuns(2)),
					expected: 8,
				},
				{
					runs:     combine(unexpectedRuns(9), expectedRuns(1)),
					expected: 9,
				},
				{
					runs:     unexpectedRuns(10),
					expected: 10,
				},
				{
					runs:     combine(expectedRuns(1), unexpectedRuns(9)),
					expected: 9,
				},
				{
					runs:     combine(expectedRuns(2), unexpectedRuns(8)),
					expected: 8,
				},
				{
					runs:     combine(expectedRuns(3), unexpectedRuns(7)),
					expected: 7,
				},
				{
					runs:     combine(expectedRuns(4), unexpectedRuns(6)),
					expected: 6,
				},
				{
					runs:     combine(expectedRuns(5), unexpectedRuns(5)),
					expected: 0,
				},
			}

			for _, tc := range testCases {
				afterRuns := tc.runs[:4]
				onRuns := tc.runs[4:6]
				beforeRuns := tc.runs[6:]
				assert.Loosely(t, consecutiveUnexpectedCount(afterRuns, onRuns, beforeRuns), should.Equal(tc.expected))
			}
		})
		t.Run("Consecutive runs do not touch start or end", func(t *ftt.Test) {
			runs := combine(expectedRuns(1), unexpectedRuns(8), expectedRuns(1))

			afterRuns := runs[:4]
			onRuns := runs[4:6]
			beforeRuns := runs[6:]
			assert.Loosely(t, consecutiveUnexpectedCount(afterRuns, onRuns, beforeRuns), should.BeZero)
		})
		t.Run("Consecutive unexpected runs on after side of queried position, no runs on queried position", func(t *ftt.Test) {
			runs := combine(unexpectedRuns(5), expectedRuns(5))

			afterRuns := runs[:5]
			onRuns := runs[5:5] // Empty slice
			beforeRuns := runs[5:]
			assert.Loosely(t, consecutiveUnexpectedCount(afterRuns, onRuns, beforeRuns), should.Equal(5))
		})
		t.Run("Consecutive unexpected runs on before side of queried position, no runs on queried position", func(t *ftt.Test) {
			runs := combine(expectedRuns(5), unexpectedRuns(5))

			afterRuns := runs[:5]
			onRuns := runs[5:5] // Empty slice
			beforeRuns := runs[5:]
			assert.Loosely(t, consecutiveUnexpectedCount(afterRuns, onRuns, beforeRuns), should.Equal(5))
		})
	})
	ftt.Run("unexpectedRunsInWindow", t, func(t *ftt.Test) {
		t.Run("no runs", func(t *ftt.Test) {
			assert.Loosely(t, unexpectedRunsInWindow(nil, 10), should.BeZero)
		})
		t.Run("fewer runs than window size", func(t *ftt.Test) {
			runs := combine(unexpectedRuns(3), expectedRuns(2), unexpectedRuns(2))
			assert.Loosely(t, unexpectedRunsInWindow(runs, 10), should.Equal(5))
		})
		t.Run("only expected runs", func(t *ftt.Test) {
			runs := expectedRuns(20)
			assert.Loosely(t, unexpectedRunsInWindow(runs, 10), should.BeZero)
		})
		t.Run("only unexpected runs", func(t *ftt.Test) {
			runs := unexpectedRuns(20)
			assert.Loosely(t, unexpectedRunsInWindow(runs, 10), should.Equal(10))
		})
		t.Run("mixed runs", func(t *ftt.Test) {
			runs := combine(expectedRuns(5), unexpectedRuns(9), expectedRuns(6))
			assert.Loosely(t, unexpectedRunsInWindow(runs, 10), should.Equal(9))
		})
		t.Run("mixed runs 2", func(t *ftt.Test) {
			runs := combine(expectedRuns(1), unexpectedRuns(4), expectedRuns(3), unexpectedRuns(4), expectedRuns(9))
			assert.Loosely(t, unexpectedRunsInWindow(runs, 10), should.Equal(7))
		})
	})
	ftt.Run("Bucket Analzyer", t, func(t *ftt.Test) {
		buckets := []*sourcePositionBucket{
			{
				StartSourcePosition:   2,
				EndSourcePosition:     3,
				EarliestPartitionTime: time.Date(2100, time.July, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				StartSourcePosition:   4,
				EndSourcePosition:     6,
				EarliestPartitionTime: time.Date(2100, time.July, 3, 0, 0, 0, 0, time.UTC),
			},
			{
				StartSourcePosition:   8,
				EndSourcePosition:     8,
				EarliestPartitionTime: time.Date(2100, time.July, 6, 0, 0, 0, 0, time.UTC),
			},
			{
				StartSourcePosition:   10,
				EndSourcePosition:     13,
				EarliestPartitionTime: time.Date(2100, time.July, 8, 0, 0, 0, 0, time.UTC),
			},
			{
				StartSourcePosition:   18,
				EndSourcePosition:     32,
				EarliestPartitionTime: time.Date(2100, time.July, 14, 0, 0, 0, 0, time.UTC),
				// Earliest availability: July 12th (due to bucket below).
			},
			{
				StartSourcePosition:   34,
				EndSourcePosition:     40,
				EarliestPartitionTime: time.Date(2100, time.July, 12, 0, 0, 0, 0, time.UTC),
			},
		}
		t.Run("With buckets", func(t *ftt.Test) {
			ba := newBucketAnalyzer(buckets)
			t.Run("query at end", func(t *ftt.Test) {
				ts := ba.earliestPartitionTimeAtSourcePosition(40)
				assert.That(t, ts, should.Match(time.Date(2100, time.July, 12, 0, 0, 0, 0, time.UTC)))

				result := ba.bucketsForTimeRange(ts.Add(-7*24*time.Hour), ts.Add(7*24*time.Hour))
				assert.Loosely(t, result, should.Resemble(buckets[2:]))
			})
			t.Run("query beyond end", func(t *ftt.Test) {
				ts := ba.earliestPartitionTimeAtSourcePosition(50)
				assert.That(t, ts, should.Match(time.Date(2100, time.July, 12, 0, 0, 0, 0, time.UTC)))
			})
			t.Run("query in middle", func(t *ftt.Test) {
				ts := ba.earliestPartitionTimeAtSourcePosition(8)
				assert.That(t, ts, should.Match(time.Date(2100, time.July, 6, 0, 0, 0, 0, time.UTC)))

				result := ba.bucketsForTimeRange(ts.Add(-7*24*time.Hour), ts.Add(7*24*time.Hour))
				assert.Loosely(t, result, should.Resemble(buckets))
			})
			t.Run("query at start", func(t *ftt.Test) {
				ts := ba.earliestPartitionTimeAtSourcePosition(2)
				assert.That(t, ts, should.Match(time.Date(2100, time.July, 1, 0, 0, 0, 0, time.UTC)))

				result := ba.bucketsForTimeRange(ts.Add(-7*24*time.Hour), ts.Add(7*24*time.Hour))
				assert.Loosely(t, result, should.Resemble(buckets[:4]))
			})
			t.Run("query before start", func(t *ftt.Test) {
				ts := ba.earliestPartitionTimeAtSourcePosition(1)
				assert.That(t, ts, should.Match(time.Date(2100, time.July, 1, 0, 0, 0, 0, time.UTC)))
			})
		})
		t.Run("Without no buckets", func(t *ftt.Test) {
			ba := newBucketAnalyzer(nil)
			ts := ba.earliestPartitionTimeAtSourcePosition(2)
			assert.That(t, ts, should.Match(time.Time{}))

			result := ba.bucketsForTimeRange(ts.Add(-7*24*time.Hour), ts.Add(7*24*time.Hour))
			assert.Loosely(t, result, should.HaveLength(0))
		})
	})
	ftt.Run("jumpBack24WeekdayHours", t, func(t *ftt.Test) {
		// Expect jumpBack24WeekdayHours to go back in time just far enough
		// that 24 workday hours are between the returned time and now.
		t.Run("Monday", func(t *ftt.Test) {
			// Given an input on a Monday (e.g. 14th of March 2022), expect
			// jumpBack24WeekdayHours to return the corresponding time
			// on the previous Friday.

			now := time.Date(2022, time.March, 14, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpBack24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(time.Date(2022, time.March, 11, 23, 59, 59, 999999999, time.UTC)))

			now = time.Date(2022, time.March, 14, 0, 0, 0, 0, time.UTC)
			afterTime = jumpBack24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(time.Date(2022, time.March, 11, 0, 0, 0, 0, time.UTC)))
		})
		t.Run("Sunday", func(t *ftt.Test) {
			// Given a time on a Sunday (e.g. 13th of March 2022), expect
			// jumpBack24WeekdayHours to return the start of the previous
			// Friday.
			startOfFriday := time.Date(2022, time.March, 11, 0, 0, 0, 0, time.UTC)

			now := time.Date(2022, time.March, 13, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpBack24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(startOfFriday))

			now = time.Date(2022, time.March, 13, 0, 0, 0, 0, time.UTC)
			afterTime = jumpBack24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(startOfFriday))
		})
		t.Run("Saturday", func(t *ftt.Test) {
			// Given a time on a Saturday (e.g. 12th of March 2022), expect
			// jumpBack24WeekdayHours to return the start of the previous
			// Friday.
			startOfFriday := time.Date(2022, time.March, 11, 0, 0, 0, 0, time.UTC)

			now := time.Date(2022, time.March, 12, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpBack24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(startOfFriday))

			now = time.Date(2022, time.March, 12, 0, 0, 0, 0, time.UTC)
			afterTime = jumpBack24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(startOfFriday))
		})
		t.Run("Tuesday to Friday", func(t *ftt.Test) {
			// Given an input on a Tuesday (e.g. 15th of March 2022), expect
			// jumpBack24WeekdayHours to return the corresponding time
			// the previous day.
			now := time.Date(2022, time.March, 15, 1, 2, 3, 4, time.UTC)
			afterTime := jumpBack24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(time.Date(2022, time.March, 14, 1, 2, 3, 4, time.UTC)))

			// Given an input on a Friday (e.g. 18th of March 2022), expect
			// jumpBack24WeekdayHours to return the corresponding time
			// the previous day.
			now = time.Date(2022, time.March, 18, 1, 2, 3, 4, time.UTC)
			afterTime = jumpBack24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(time.Date(2022, time.March, 17, 1, 2, 3, 4, time.UTC)))
		})
	})
	ftt.Run("jumpAhead24WeekdayHours", t, func(t *ftt.Test) {
		// Expect jumpAhead24WeekdayHours to go forward in time far enough
		// that 24 workday hours are between the returned time and now.
		t.Run("Friday", func(t *ftt.Test) {
			// Given an input on a Friday (e.g. 11th of March 2022), expect
			// jumpForward24WeekdayHours to return the corresponding time
			// on the next Monday.

			now := time.Date(2022, time.March, 11, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpAhead24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(time.Date(2022, time.March, 14, 23, 59, 59, 999999999, time.UTC)))

			now = time.Date(2022, time.March, 11, 0, 0, 0, 0, time.UTC)
			afterTime = jumpAhead24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(time.Date(2022, time.March, 14, 0, 0, 0, 0, time.UTC)))
		})
		t.Run("Sunday", func(t *ftt.Test) {
			// Given a time on a Sunday (e.g. 13th of March 2022), expect
			// jumpAhead24WeekdayHours to return the start of the next
			// Tuesday.
			startOfTuesday := time.Date(2022, time.March, 15, 0, 0, 0, 0, time.UTC)

			now := time.Date(2022, time.March, 13, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpAhead24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(startOfTuesday))

			now = time.Date(2022, time.March, 13, 0, 0, 0, 0, time.UTC)
			afterTime = jumpAhead24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(startOfTuesday))
		})
		t.Run("Saturday", func(t *ftt.Test) {
			// Given a time on a Saturday (e.g. 12th of March 2022), expect
			// jumpAhead24WeekdayHours to return the start of the next
			// Tuesday.
			startOfTuesday := time.Date(2022, time.March, 15, 0, 0, 0, 0, time.UTC)

			now := time.Date(2022, time.March, 12, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpAhead24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(startOfTuesday))

			now = time.Date(2022, time.March, 12, 0, 0, 0, 0, time.UTC)
			afterTime = jumpAhead24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(startOfTuesday))
		})
		t.Run("Monday to Thursday", func(t *ftt.Test) {
			// Given an input on a Monday (e.g. 14th of March 2022), expect
			// jumpAhead24WeekdayHours to return the corresponding time
			// the next day.
			now := time.Date(2022, time.March, 14, 1, 2, 3, 4, time.UTC)
			afterTime := jumpAhead24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(time.Date(2022, time.March, 15, 1, 2, 3, 4, time.UTC)))

			// Given an input on a Thursday (e.g. 17th of March 2022), expect
			// jumpAhead24WeekdayHours to return the corresponding time
			// the next day.
			now = time.Date(2022, time.March, 17, 1, 2, 3, 4, time.UTC)
			afterTime = jumpAhead24WeekdayHours(now)
			assert.That(t, afterTime, should.Match(time.Date(2022, time.March, 18, 1, 2, 3, 4, time.UTC)))
		})
	})
}

func unexpectedRuns(count int) []run {
	var result []run
	for range count {
		result = append(result, run{expected: false})
	}
	return result
}

func expectedRuns(count int) []run {
	var result []run
	for range count {
		result = append(result, run{expected: true})
	}
	return result
}

func combine(runs ...[]run) []run {
	var result []run
	for _, runs := range runs {
		result = append(result, runs...)
	}
	return result
}
