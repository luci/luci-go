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

package testresults

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

func TestQueryFailureRate(t *testing.T) {
	Convey("QueryFailureRate", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		var1 := pbutil.Variant("key1", "val1", "key2", "val1")
		var3 := pbutil.Variant("key1", "val2", "key2", "val2")

		err := CreateQueryFailureRateTestData(ctx)
		So(err, ShouldBeNil)

		project, asAtTime, tvs := QueryFailureRateSampleRequest()
		opts := QueryFailureRateOptions{
			Project:      project,
			SubRealms:    []string{"realm"},
			TestVariants: tvs,
			AsAtTime:     asAtTime,
		}
		expectedResult := QueryFailureRateSampleResponse()
		txn, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		Convey("Baseline", func() {
			result, err := QueryFailureRate(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
		})
		Convey("Project filter works correctly", func() {
			opts.Project = "none"
			expectedResult.TestVariants = []*pb.TestVariantFailureRateAnalysis{
				emptyAnalysis("test_id", var1),
				emptyAnalysis("test_id", var3),
			}

			result, err := QueryFailureRate(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
		})
		Convey("Realm filter works correctly", func() {
			// No data exists in this realm.
			opts.SubRealms = []string{"otherrealm"}
			expectedResult.TestVariants = []*pb.TestVariantFailureRateAnalysis{
				emptyAnalysis("test_id", var1),
				emptyAnalysis("test_id", var3),
			}

			result, err := QueryFailureRate(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
		})
		Convey("Works for tests without data", func() {
			notExistsVariant := pbutil.Variant("key1", "val1", "key2", "not_exists")
			opts.TestVariants = append(opts.TestVariants,
				&pb.TestVariantIdentifier{
					TestId:  "not_exists_test_id",
					Variant: var1,
				},
				&pb.TestVariantIdentifier{
					TestId:  "test_id",
					Variant: notExistsVariant,
				})

			expectedResult.TestVariants = append(expectedResult.TestVariants,
				emptyAnalysis("not_exists_test_id", var1),
				emptyAnalysis("test_id", notExistsVariant))

			result, err := QueryFailureRate(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
		})
		Convey("Batching works correctly", func() {
			// Ensure the order of test variants in the request and response
			// remain correct even when there are multiple batches.
			var expandedInput []*pb.TestVariantIdentifier
			var expectedOutput []*pb.TestVariantFailureRateAnalysis
			for i := 0; i < batchSize; i++ {
				testID := fmt.Sprintf("test_id_%v", i)
				expandedInput = append(expandedInput, &pb.TestVariantIdentifier{
					TestId:  testID,
					Variant: var1,
				})
				expectedOutput = append(expectedOutput, emptyAnalysis(testID, var1))
			}

			expandedInput = append(expandedInput, tvs...)
			expectedOutput = append(expectedOutput, expectedResult.TestVariants...)

			opts.TestVariants = expandedInput
			expectedResult.TestVariants = expectedOutput

			result, err := QueryFailureRate(txn, opts)
			So(err, ShouldBeNil)
			So(result, ShouldResembleProto, expectedResult)
		})
	})
}

// emptyAnalysis returns an empty analysis proto with intervals populated.
func emptyAnalysis(testId string, variant *pb.Variant) *pb.TestVariantFailureRateAnalysis {
	return &pb.TestVariantFailureRateAnalysis{
		TestId:  testId,
		Variant: variant,
		IntervalStats: []*pb.TestVariantFailureRateAnalysis_IntervalStats{
			{IntervalAge: 1},
			{IntervalAge: 2},
			{IntervalAge: 3},
			{IntervalAge: 4},
			{IntervalAge: 5},
		},
		RunFlakyVerdictExamples: []*pb.TestVariantFailureRateAnalysis_VerdictExample{},
		RecentVerdicts:          []*pb.TestVariantFailureRateAnalysis_RecentVerdict{},
	}
}

func TestJumpBack24WeekdayHours(t *testing.T) {
	Convey("jumpBack24WeekdayHours", t, func() {
		// Expect jumpBack24WeekdayHours to go back in time just far enough
		// that 24 workday hours are between the returned time and now.
		Convey("Monday", func() {
			// Given an input on a Monday (e.g. 14th of March 2022), expect
			// failureRateQueryAfterTime to return the corresponding time
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
			// failureRateQueryAfterTime to return the start of the previous
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
			// failureRateQueryAfterTime to return the start of the previous
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
			// failureRateQueryAfterTime to return the corresponding time
			// the previous day.
			now := time.Date(2022, time.March, 15, 1, 2, 3, 4, time.UTC)
			afterTime := jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 14, 1, 2, 3, 4, time.UTC))

			// Given an input on a Friday (e.g. 18th of March 2022), expect
			// failureRateQueryAfterTime to return the corresponding time
			// the previous day.
			now = time.Date(2022, time.March, 18, 1, 2, 3, 4, time.UTC)
			afterTime = jumpBack24WeekdayHours(now)
			So(afterTime, ShouldEqual, time.Date(2022, time.March, 17, 1, 2, 3, 4, time.UTC))
		})
	})
}
