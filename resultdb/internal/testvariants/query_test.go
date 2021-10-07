// Copyright 2020 The LUCI Authors.
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
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueryTestVariants(t *testing.T) {
	Convey(`QueryTestVariants`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		q := &Query{
			InvocationIDs: invocations.NewIDSet("inv0", "inv1"),
			PageSize:      100,
		}

		fetch := func(q *Query) (tvs []*pb.TestVariant, token string, err error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return q.Fetch(ctx)
		}

		mustFetch := func(q *Query) (tvs []*pb.TestVariant, token string) {
			tvs, token, err := fetch(q)
			So(err, ShouldBeNil)
			return
		}

		getTVStrings := func(tvs []*pb.TestVariant) []string {
			tvStrings := make([]string, len(tvs))
			for i, tv := range tvs {
				tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
			}
			return tvStrings
		}

		testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, insert.Invocation("inv1", pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.TestResults("inv0", "T1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
			insert.TestResults("inv0", "T2", nil, pb.TestStatus_PASS),
			insert.TestResults("inv0", "T5", nil, pb.TestStatus_FAIL),
			insert.TestResults(
				"inv0", "T6", nil,
				pb.TestStatus_PASS, pb.TestStatus_PASS, pb.TestStatus_PASS,
				pb.TestStatus_PASS, pb.TestStatus_PASS, pb.TestStatus_PASS,
				pb.TestStatus_PASS, pb.TestStatus_PASS, pb.TestStatus_PASS,
				pb.TestStatus_PASS, pb.TestStatus_PASS, pb.TestStatus_PASS,
			),
			insert.TestResults("inv0", "T7", nil, pb.TestStatus_PASS),
			insert.TestResults("inv0", "T8", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
			insert.TestResults("inv0", "T9", nil, pb.TestStatus_PASS),
			insert.TestResults("inv1", "T1", nil, pb.TestStatus_PASS),
			insert.TestResults("inv1", "T2", nil, pb.TestStatus_FAIL),
			insert.TestResults("inv1", "T3", nil, pb.TestStatus_PASS, pb.TestStatus_PASS),
			insert.TestResults("inv1", "T5", pbutil.Variant("a", "b"), pb.TestStatus_FAIL, pb.TestStatus_PASS),
			insert.TestResults(
				"inv1", "Ty", nil,
				pb.TestStatus_FAIL, pb.TestStatus_FAIL, pb.TestStatus_FAIL,
				pb.TestStatus_FAIL, pb.TestStatus_FAIL, pb.TestStatus_FAIL,
				pb.TestStatus_FAIL, pb.TestStatus_FAIL, pb.TestStatus_FAIL,
				pb.TestStatus_FAIL, pb.TestStatus_FAIL, pb.TestStatus_FAIL,
			),
			insert.TestResults("inv1", "Tx", nil, pb.TestStatus_SKIP),
			insert.TestResults("inv1", "Tz", nil, pb.TestStatus_SKIP, pb.TestStatus_SKIP),

			insert.TestExonerations("inv0", "T1", nil, 1),
		)...)

		// Insert an additional TestResult for comparing TestVariant.Results.Result.
		startTime := timestamppb.New(testclock.TestRecentTimeUTC.Add(-2 * time.Minute))
		duration := &durationpb.Duration{Seconds: 0, Nanos: 234567000}
		strPairs := pbutil.StringPairs(
			"buildername", "blder",
			"test_suite", "foo_unittests",
			"test_id_prefix", "ninja://tests:tests/")

		tmd := &pb.TestMetadata{
			Name: "T4",
			Location: &pb.TestLocation{
				FileName: "//t4.go",
				Line:     54,
			}}
		tmdBytes, _ := proto.Marshal(tmd)

		failureReason := &pb.FailureReason{
			PrimaryErrorMessage: "primary error msg",
		}
		failureReasonBytes, _ := proto.Marshal(failureReason)

		testutil.MustApply(ctx,
			spanutil.InsertMap("TestResults", map[string]interface{}{
				"InvocationId":    invocations.ID("inv1"),
				"TestId":          "T4",
				"ResultId":        "0",
				"Variant":         pbutil.Variant("a", "b"),
				"VariantHash":     pbutil.VariantHash(pbutil.Variant("a", "b")),
				"CommitTimestamp": spanner.CommitTimestamp,
				"IsUnexpected":    true,
				"Status":          pb.TestStatus_FAIL,
				"RunDurationUsec": pbutil.MustDuration(duration).Microseconds(),
				"StartTime":       startTime,
				"SummaryHtml":     spanutil.Compressed("SummaryHtml"),
				"FailureReason":   spanutil.Compressed(failureReasonBytes),
				"Tags":            pbutil.StringPairsToStrings(strPairs...),
				"TestMetadata":    spanutil.Compressed(tmdBytes),
			}),
		)

		// Tx has an expected skip so it should be FLAKY instead of UNEXPECTEDLY_SKIPPED.
		testutil.MustApply(ctx,
			spanutil.InsertMap("TestResults", map[string]interface{}{
				"InvocationId":    invocations.ID("inv1"),
				"TestId":          "Tx",
				"ResultId":        "1",
				"Variant":         nil,
				"VariantHash":     pbutil.VariantHash(nil),
				"CommitTimestamp": spanner.CommitTimestamp,
				"IsUnexpected":    false,
				"Status":          pb.TestStatus_SKIP,
				"RunDurationUsec": pbutil.MustDuration(duration).Microseconds(),
				"StartTime":       startTime,
			}),
		)

		Convey(`Unexpected works`, func() {
			tvs, _ := mustFetch(q)
			tvStrings := getTVStrings(tvs)
			So(tvStrings, ShouldResemble, []string{
				"10/T4/c467ccce5a16dc72",
				"10/T5/e3b0c44298fc1c14",
				"10/Ty/e3b0c44298fc1c14",
				"20/Tz/e3b0c44298fc1c14",
				"30/T2/e3b0c44298fc1c14",
				"30/T5/c467ccce5a16dc72",
				"30/T8/e3b0c44298fc1c14",
				"30/Tx/e3b0c44298fc1c14",
				"40/T1/e3b0c44298fc1c14",
			})

			So(tvs[0].Results, ShouldResembleProto, []*pb.TestResultBundle{
				&pb.TestResultBundle{
					Result: &pb.TestResult{
						Name:        "invocations/inv1/tests/T4/results/0",
						ResultId:    "0",
						Expected:    false,
						Status:      pb.TestStatus_FAIL,
						StartTime:   startTime,
						Duration:    duration,
						SummaryHtml: "SummaryHtml",
						FailureReason: &pb.FailureReason{
							PrimaryErrorMessage: "primary error msg",
						},
						Tags: strPairs,
					},
				},
			})
			So(tvs[0].TestMetadata, ShouldResembleProto, tmd)
			So(tvs[8].Exonerations[0], ShouldResemble, &pb.TestExoneration{
				ExplanationHtml: "explanation 0",
			})
			So(len(tvs[2].Results), ShouldEqual, 10)
		})

		Convey(`Expected works`, func() {
			q.PageToken = pagination.Token("EXPECTED", "", "")
			tvs, _ := mustFetch(q)
			So(getTVStrings(tvs), ShouldResemble, []string{
				"50/T3/e3b0c44298fc1c14",
				"50/T6/e3b0c44298fc1c14",
				"50/T7/e3b0c44298fc1c14",
				"50/T9/e3b0c44298fc1c14",
			})
			So(len(tvs[0].Results), ShouldEqual, 2)
		})

		Convey(`paging works`, func() {
			page := func(token string, expectedTVLen int32, expectedTVStrings []string) string {
				q.PageToken = token
				tvs, nextToken := mustFetch(q)
				So(getTVStrings(tvs), ShouldResemble, expectedTVStrings)
				return nextToken
			}

			q.PageSize = 15
			nextToken := page("", 8, []string{
				"10/T4/c467ccce5a16dc72",
				"10/T5/e3b0c44298fc1c14",
				"10/Ty/e3b0c44298fc1c14",
				"20/Tz/e3b0c44298fc1c14",
				"30/T2/e3b0c44298fc1c14",
				"30/T5/c467ccce5a16dc72",
				"30/T8/e3b0c44298fc1c14",
				"30/Tx/e3b0c44298fc1c14",
				"40/T1/e3b0c44298fc1c14",
			})
			So(nextToken, ShouldEqual, pagination.Token("EXPECTED", "", ""))

			nextToken = page(nextToken, 1, []string{
				"50/T3/e3b0c44298fc1c14",
			})
			So(nextToken, ShouldEqual, pagination.Token("EXPECTED", "T5", "e3b0c44298fc1c14"))

			nextToken = page(nextToken, 2, []string{
				"50/T6/e3b0c44298fc1c14",
				"50/T7/e3b0c44298fc1c14",
			})
			So(nextToken, ShouldEqual, pagination.Token("EXPECTED", "T8", "e3b0c44298fc1c14"))

			nextToken = page(nextToken, 1, []string{"50/T9/e3b0c44298fc1c14"})
			So(nextToken, ShouldEqual, "CghFWFBFQ1RFRAoCVHoKEGUzYjBjNDQyOThmYzFjMTQ=")

			nextToken = page(nextToken, 0, []string{})
			So(nextToken, ShouldEqual, "")
		})

		Convey(`Page Token`, func() {
			Convey(`wrong number of parts`, func() {
				q.PageToken = pagination.Token("testId", "variantHash")
				_, _, err := q.Fetch(ctx)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
			})

			Convey(`first part not tvStatus`, func() {
				q.PageToken = pagination.Token("50", "testId", "variantHash")
				_, _, err := q.Fetch(ctx)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
			})
		})

		Convey(`status filter works`, func() {
			Convey(`only unexpected`, func() {
				q.Predicate = &pb.TestVariantPredicate{Status: pb.TestVariantStatus_UNEXPECTED}
				tvs, token := mustFetch(q)
				tvStrings := getTVStrings(tvs)
				So(tvStrings, ShouldResemble, []string{
					"10/T4/c467ccce5a16dc72",
					"10/T5/e3b0c44298fc1c14",
					"10/Ty/e3b0c44298fc1c14",
				})
				So(token, ShouldEqual, "")
			})

			Convey(`only expected`, func() {
				q.Predicate = &pb.TestVariantPredicate{Status: pb.TestVariantStatus_EXPECTED}
				tvs, _ := mustFetch(q)
				So(getTVStrings(tvs), ShouldResemble, []string{
					"50/T3/e3b0c44298fc1c14",
					"50/T6/e3b0c44298fc1c14",
					"50/T7/e3b0c44298fc1c14",
					"50/T9/e3b0c44298fc1c14",
				})
				So(len(tvs[0].Results), ShouldEqual, 2)
			})

			Convey(`any unexpected or exonerated`, func() {
				q.Predicate = &pb.TestVariantPredicate{Status: pb.TestVariantStatus_UNEXPECTED_MASK}
				tvs, _ := mustFetch(q)
				So(getTVStrings(tvs), ShouldResemble, []string{
					"10/T4/c467ccce5a16dc72",
					"10/T5/e3b0c44298fc1c14",
					"10/Ty/e3b0c44298fc1c14",
					"20/Tz/e3b0c44298fc1c14",
					"30/T2/e3b0c44298fc1c14",
					"30/T5/c467ccce5a16dc72",
					"30/T8/e3b0c44298fc1c14",
					"30/Tx/e3b0c44298fc1c14",
					"40/T1/e3b0c44298fc1c14",
				})
			})
		})
	})
}
