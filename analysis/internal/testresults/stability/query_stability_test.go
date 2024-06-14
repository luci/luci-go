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

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueryStability(t *testing.T) {
	Convey("QueryStability", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		var1 := pbutil.Variant("key1", "val1", "key2", "val1")
		var3 := pbutil.Variant("key1", "val2", "key2", "val2")

		err := CreateQueryStabilityTestData(ctx)
		So(err, ShouldBeNil)

		opts := QueryStabilitySampleRequest()
		expectedResult := QueryStabilitySampleResponse()
		txn, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		Convey("Baseline", func() {
			result, err := QueryStability(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
		})
		Convey("Flake analysis uses full 14 days if MinWindow unmet", func() {
			opts.Criteria.FlakeRate.MinWindow = 100
			result, err := QueryStability(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, QueryStabilitySampleResponseLargeWindow())
		})
		Convey("Project filter works correctly", func() {
			opts.Project = "none"
			expectedResult = []*pb.TestVariantStabilityAnalysis{
				emptyStabilityAnalysis("test_id", var1),
				emptyStabilityAnalysis("test_id", var3),
			}

			result, err := QueryStability(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
		})
		Convey("Realm filter works correctly", func() {
			// No data exists in this realm.
			opts.SubRealms = []string{"otherrealm"}
			expectedResult = []*pb.TestVariantStabilityAnalysis{
				emptyStabilityAnalysis("test_id", var1),
				emptyStabilityAnalysis("test_id", var3),
			}

			result, err := QueryStability(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
		})
		Convey("Works for tests without data", func() {
			notExistsVariant := pbutil.Variant("key1", "val1", "key2", "not_exists")
			opts.TestVariantPositions = append(opts.TestVariantPositions,
				&pb.QueryTestVariantStabilityRequest_TestVariantPosition{
					TestId:  "not_exists_test_id",
					Variant: var1,
					Sources: &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{
							Host:       "mysources.googlesource.com",
							Project:    "myproject/src",
							Ref:        "refs/heads/mybranch",
							CommitHash: "aabbccddeeff00112233aabbccddeeff00112233",
							Position:   130,
						},
					},
				},
				&pb.QueryTestVariantStabilityRequest_TestVariantPosition{
					TestId:  "test_id",
					Variant: notExistsVariant,
					Sources: &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{
							Host:       "mysources.googlesource.com",
							Project:    "myproject/src",
							Ref:        "refs/heads/mybranch",
							CommitHash: "aabbccddeeff00112233aabbccddeeff00112233",
							Position:   130,
						},
					},
				})

			expectedResult = append(expectedResult,
				emptyStabilityAnalysis("not_exists_test_id", var1),
				emptyStabilityAnalysis("test_id", notExistsVariant))

			result, err := QueryStability(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
		})
		Convey("Batching works correctly", func() {
			// Ensure the order of test variants in the request and response
			// remain correct even when there are multiple batches.
			var expandedInput []*pb.QueryTestVariantStabilityRequest_TestVariantPosition
			var expectedOutput []*pb.TestVariantStabilityAnalysis
			for i := 0; i < batchSize; i++ {
				testID := fmt.Sprintf("test_id_%v", i)
				expandedInput = append(expandedInput, &pb.QueryTestVariantStabilityRequest_TestVariantPosition{
					TestId:  testID,
					Variant: var1,
					Sources: &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{
							Host:       "mysources.googlesource.com",
							Project:    "myproject/src",
							Ref:        "refs/heads/mybranch",
							CommitHash: "aabbccddeeff00112233aabbccddeeff00112233",
							Position:   130,
						},
					},
				})
				expectedOutput = append(expectedOutput, emptyStabilityAnalysis(testID, var1))
			}

			opts.TestVariantPositions = append(expandedInput, opts.TestVariantPositions...)
			expectedResult = append(expectedOutput, expectedResult...)

			result, err := QueryStability(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
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
	Convey("flattenSourceVerdictsToRuns", t, func() {
		unexpectedRun := run{expected: false}
		expectedRun := run{expected: true}
		Convey("With no verdicts", func() {
			verdicts := []*sourceVerdict{}
			result := flattenSourceVerdictsToRuns(verdicts)
			So(result, ShouldHaveLength, 0)
		})
		Convey("With one unexpected run", func() {
			verdicts := []*sourceVerdict{
				{
					UnexpectedRuns: 1,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			So(result, ShouldResemble, []run{unexpectedRun})
		})
		Convey("With many unexpected runs", func() {
			verdicts := []*sourceVerdict{
				{
					UnexpectedRuns: 3,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			So(result, ShouldResemble, []run{unexpectedRun, unexpectedRun, unexpectedRun})
		})
		Convey("With one expected run", func() {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns: 1,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			So(result, ShouldResemble, []run{expectedRun})
		})
		Convey("With many expected run", func() {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns: 3,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			So(result, ShouldResemble, []run{expectedRun, expectedRun, expectedRun})
		})
		Convey("With mixed runs, evenly split", func() {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns:   3,
					UnexpectedRuns: 3,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			So(result, ShouldResemble, []run{unexpectedRun, expectedRun, unexpectedRun, expectedRun, unexpectedRun, expectedRun})
		})
		Convey("With mixed runs, 3/2 split", func() {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns:   4,
					UnexpectedRuns: 2,
				},
			}
			result := flattenSourceVerdictsToRuns(verdicts)
			So(result, ShouldResemble, []run{unexpectedRun, expectedRun, expectedRun, unexpectedRun, expectedRun, expectedRun})
		})
		Convey("With multiple verdicts", func() {
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
			So(result, ShouldResemble, []run{expectedRun, expectedRun, unexpectedRun, expectedRun, unexpectedRun, unexpectedRun})
		})
	})
	Convey("truncateSourceVerdicts", t, func() {
		Convey("With no verdicts", func() {
			verdicts := []*sourceVerdict{}
			result := truncateSourceVerdicts(verdicts, 10)
			So(result, ShouldHaveLength, 0)
		})
		Convey("With large expected verdict", func() {
			verdicts := []*sourceVerdict{
				{
					ExpectedRuns: 11,
				},
			}
			result := truncateSourceVerdicts(verdicts, 10)
			So(result, ShouldResemble, []*sourceVerdict{
				{
					ExpectedRuns: 10,
				},
			})
		})
		Convey("With large unexpected verdict", func() {
			verdicts := []*sourceVerdict{
				{
					UnexpectedRuns: 111,
				},
			}
			result := truncateSourceVerdicts(verdicts, 10)
			So(result, ShouldResemble, []*sourceVerdict{
				{
					UnexpectedRuns: 10,
				},
			})
		})
		Convey("With multiple verdicts", func() {
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
			So(result, ShouldResemble, []*sourceVerdict{
				{
					ExpectedRuns: 2,
				},
				{
					ExpectedRuns:   4,
					UnexpectedRuns: 4,
				},
			})
		})
	})
	Convey("consecutiveFailureCount", t, func() {
		Convey("Consecutive from start and/or end", func() {
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
				So(consecutiveUnexpectedCount(afterRuns, onRuns, beforeRuns), ShouldEqual, tc.expected)
			}
		})
		Convey("Consecutive runs do not touch start or end", func() {
			runs := combine(expectedRuns(1), unexpectedRuns(8), expectedRuns(1))

			afterRuns := runs[:4]
			onRuns := runs[4:6]
			beforeRuns := runs[6:]
			So(consecutiveUnexpectedCount(afterRuns, onRuns, beforeRuns), ShouldEqual, 0)
		})
		Convey("Consecutive unexpected runs on after side of queried position, no runs on queried position", func() {
			runs := combine(unexpectedRuns(5), expectedRuns(5))

			afterRuns := runs[:5]
			onRuns := runs[5:5] // Empty slice
			beforeRuns := runs[5:]
			So(consecutiveUnexpectedCount(afterRuns, onRuns, beforeRuns), ShouldEqual, 5)
		})
		Convey("Consecutive unexpected runs on before side of queried position, no runs on queried position", func() {
			runs := combine(expectedRuns(5), unexpectedRuns(5))

			afterRuns := runs[:5]
			onRuns := runs[5:5] // Empty slice
			beforeRuns := runs[5:]
			So(consecutiveUnexpectedCount(afterRuns, onRuns, beforeRuns), ShouldEqual, 5)
		})
	})
	Convey("unexpectedRunsInWindow", t, func() {
		Convey("no runs", func() {
			So(unexpectedRunsInWindow(nil, 10), ShouldEqual, 0)
		})
		Convey("fewer runs than window size", func() {
			runs := combine(unexpectedRuns(3), expectedRuns(2), unexpectedRuns(2))
			So(unexpectedRunsInWindow(runs, 10), ShouldEqual, 5)
		})
		Convey("only expected runs", func() {
			runs := expectedRuns(20)
			So(unexpectedRunsInWindow(runs, 10), ShouldEqual, 0)
		})
		Convey("only unexpected runs", func() {
			runs := unexpectedRuns(20)
			So(unexpectedRunsInWindow(runs, 10), ShouldEqual, 10)
		})
		Convey("mixed runs", func() {
			runs := combine(expectedRuns(5), unexpectedRuns(9), expectedRuns(6))
			So(unexpectedRunsInWindow(runs, 10), ShouldEqual, 9)
		})
		Convey("mixed runs 2", func() {
			runs := combine(expectedRuns(1), unexpectedRuns(4), expectedRuns(3), unexpectedRuns(4), expectedRuns(9))
			So(unexpectedRunsInWindow(runs, 10), ShouldEqual, 7)
		})
	})
	Convey("Bucket Analzyer", t, func() {
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
		Convey("With buckets", func() {
			ba := newBucketAnalyzer(buckets)
			Convey("query at end", func() {
				t := ba.earliestPartitionTimeAtSourcePosition(40)
				So(t, ShouldEqual, time.Date(2100, time.July, 12, 0, 0, 0, 0, time.UTC))

				result := ba.bucketsForTimeRange(t.Add(-7*24*time.Hour), t.Add(7*24*time.Hour))
				So(result, ShouldResemble, buckets[2:])
			})
			Convey("query beyond end", func() {
				t := ba.earliestPartitionTimeAtSourcePosition(50)
				So(t, ShouldEqual, time.Date(2100, time.July, 12, 0, 0, 0, 0, time.UTC))
			})
			Convey("query in middle", func() {
				t := ba.earliestPartitionTimeAtSourcePosition(8)
				So(t, ShouldEqual, time.Date(2100, time.July, 6, 0, 0, 0, 0, time.UTC))

				result := ba.bucketsForTimeRange(t.Add(-7*24*time.Hour), t.Add(7*24*time.Hour))
				So(result, ShouldResemble, buckets)
			})
			Convey("query at start", func() {
				t := ba.earliestPartitionTimeAtSourcePosition(2)
				So(t, ShouldEqual, time.Date(2100, time.July, 1, 0, 0, 0, 0, time.UTC))

				result := ba.bucketsForTimeRange(t.Add(-7*24*time.Hour), t.Add(7*24*time.Hour))
				So(result, ShouldResemble, buckets[:4])
			})
			Convey("query before start", func() {
				t := ba.earliestPartitionTimeAtSourcePosition(1)
				So(t, ShouldEqual, time.Date(2100, time.July, 1, 0, 0, 0, 0, time.UTC))
			})
		})
		Convey("Without no buckets", func() {
			ba := newBucketAnalyzer(nil)
			t := ba.earliestPartitionTimeAtSourcePosition(2)
			So(t, ShouldEqual, time.Time{})

			result := ba.bucketsForTimeRange(t.Add(-7*24*time.Hour), t.Add(7*24*time.Hour))
			So(result, ShouldHaveLength, 0)
		})
	})
	Convey("jumpBack24WeekdayHours", t, func() {
		// Expect jumpBack24WeekdayHours to go back in time just far enough
		// that 24 workday hours are between the returned time and now.
		Convey("Monday", func() {
			// Given an input on a Monday (e.g. 14th of March 2022), expect
			// jumpBack24WeekdayHours to return the corresponding time
			// on the previous Friday.

			now := time.Date(2022, time.March, 14, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 11, 23, 59, 59, 999999999, time.UTC))

			now = time.Date(2022, time.March, 14, 0, 0, 0, 0, time.UTC)
			afterTime = jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 11, 0, 0, 0, 0, time.UTC))
		})
		Convey("Sunday", func() {
			// Given a time on a Sunday (e.g. 13th of March 2022), expect
			// jumpBack24WeekdayHours to return the start of the previous
			// Friday.
			startOfFriday := time.Date(2022, time.March, 11, 0, 0, 0, 0, time.UTC)

			now := time.Date(2022, time.March, 13, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, startOfFriday)

			now = time.Date(2022, time.March, 13, 0, 0, 0, 0, time.UTC)
			afterTime = jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, startOfFriday)
		})
		Convey("Saturday", func() {
			// Given a time on a Saturday (e.g. 12th of March 2022), expect
			// jumpBack24WeekdayHours to return the start of the previous
			// Friday.
			startOfFriday := time.Date(2022, time.March, 11, 0, 0, 0, 0, time.UTC)

			now := time.Date(2022, time.March, 12, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, startOfFriday)

			now = time.Date(2022, time.March, 12, 0, 0, 0, 0, time.UTC)
			afterTime = jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, startOfFriday)
		})
		Convey("Tuesday to Friday", func() {
			// Given an input on a Tuesday (e.g. 15th of March 2022), expect
			// jumpBack24WeekdayHours to return the corresponding time
			// the previous day.
			now := time.Date(2022, time.March, 15, 1, 2, 3, 4, time.UTC)
			afterTime := jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 14, 1, 2, 3, 4, time.UTC))

			// Given an input on a Friday (e.g. 18th of March 2022), expect
			// jumpBack24WeekdayHours to return the corresponding time
			// the previous day.
			now = time.Date(2022, time.March, 18, 1, 2, 3, 4, time.UTC)
			afterTime = jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 17, 1, 2, 3, 4, time.UTC))
		})
	})
	Convey("jumpAhead24WeekdayHours", t, func() {
		// Expect jumpAhead24WeekdayHours to go forward in time far enough
		// that 24 workday hours are between the returned time and now.
		Convey("Friday", func() {
			// Given an input on a Friday (e.g. 11th of March 2022), expect
			// jumpForward24WeekdayHours to return the corresponding time
			// on the next Monday.

			now := time.Date(2022, time.March, 11, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpAhead24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 14, 23, 59, 59, 999999999, time.UTC))

			now = time.Date(2022, time.March, 11, 0, 0, 0, 0, time.UTC)
			afterTime = jumpAhead24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 14, 0, 0, 0, 0, time.UTC))
		})
		Convey("Sunday", func() {
			// Given a time on a Sunday (e.g. 13th of March 2022), expect
			// jumpAhead24WeekdayHours to return the start of the next
			// Tuesday.
			startOfTuesday := time.Date(2022, time.March, 15, 0, 0, 0, 0, time.UTC)

			now := time.Date(2022, time.March, 13, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpAhead24WeekdayHours(now)
			So(afterTime, ShouldEqual, startOfTuesday)

			now = time.Date(2022, time.March, 13, 0, 0, 0, 0, time.UTC)
			afterTime = jumpAhead24WeekdayHours(now)
			So(afterTime, ShouldEqual, startOfTuesday)
		})
		Convey("Saturday", func() {
			// Given a time on a Saturday (e.g. 12th of March 2022), expect
			// jumpAhead24WeekdayHours to return the start of the next
			// Tuesday.
			startOfTuesday := time.Date(2022, time.March, 15, 0, 0, 0, 0, time.UTC)

			now := time.Date(2022, time.March, 12, 23, 59, 59, 999999999, time.UTC)
			afterTime := jumpAhead24WeekdayHours(now)
			So(afterTime, ShouldEqual, startOfTuesday)

			now = time.Date(2022, time.March, 12, 0, 0, 0, 0, time.UTC)
			afterTime = jumpAhead24WeekdayHours(now)
			So(afterTime, ShouldEqual, startOfTuesday)
		})
		Convey("Monday to Thursday", func() {
			// Given an input on a Monday (e.g. 14th of March 2022), expect
			// jumpAhead24WeekdayHours to return the corresponding time
			// the next day.
			now := time.Date(2022, time.March, 14, 1, 2, 3, 4, time.UTC)
			afterTime := jumpAhead24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 15, 1, 2, 3, 4, time.UTC))

			// Given an input on a Thursday (e.g. 17th of March 2022), expect
			// jumpAhead24WeekdayHours to return the corresponding time
			// the next day.
			now = time.Date(2022, time.March, 17, 1, 2, 3, 4, time.UTC)
			afterTime = jumpAhead24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 18, 1, 2, 3, 4, time.UTC))
		})
	})
}

func unexpectedRuns(count int) []run {
	var result []run
	for i := 0; i < count; i++ {
		result = append(result, run{expected: false})
	}
	return result
}

func expectedRuns(count int) []run {
	var result []run
	for i := 0; i < count; i++ {
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
