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
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/mask"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

func TestQueryTestVariants(t *testing.T) {
	Convey(`QueryTestVariants`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		reachableInvs := make(graph.ReachableInvocations)
		reachableInvs["inv0"] = graph.ReachableInvocation{
			HasTestResults:      true,
			HasTestExonerations: true,
		}
		reachableInvs["inv1"] = graph.ReachableInvocation{
			HasTestResults: true,
		}
		reachableInvs["inv2"] = graph.ReachableInvocation{
			HasTestExonerations: true,
		}

		q := &Query{
			ReachableInvocations: reachableInvs,
			PageSize:             100,
			ResultLimit:          10,
			ResponseLimitBytes:   DefaultResponseLimitBytes,
		}

		fetch := func(q *Query) (tvs []*pb.TestVariant, token string, err error) {
			queryAlt := &Query{}
			*queryAlt = *q

			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			tvs, token, err = q.Fetch(ctx)

			// Run the same query, with UseLargeInvocationGraphQuery = true
			// and expect it to return exactly the same results.
			queryAlt.UseLargeInvocationGraphQuery = true
			tvsB, tokenB, errB := q.Fetch(ctx)
			So(tvsB, ShouldResembleProto, tvs)
			So(tokenB, ShouldEqual, token)
			So(errB, ShouldErrLike, err)

			return tvs, token, err
		}

		mustFetch := func(q *Query) (tvs []*pb.TestVariant, token string) {
			tvs, token, err := fetch(q)
			So(err, ShouldBeNil)
			return
		}

		fetchAll := func(q *Query, f func(tvs []*pb.TestVariant)) {
			for {
				tvs, token, err := fetch(q)
				So(err, ShouldBeNil)
				if token == "" {
					break
				}
				f(tvs)

				// The page token should always advance, it should
				// never remain the same.
				So(token, ShouldNotEqual, q.PageToken)
				q.PageToken = token
			}
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
		testutil.MustApply(ctx, insert.Invocation("inv2", pb.Invocation_ACTIVE, nil))
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

			insert.TestExonerations("inv0", "T1", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
				pb.ExonerationReason_NOT_CRITICAL, pb.ExonerationReason_OCCURS_ON_MAINLINE),
			insert.TestExonerations("inv2", "T2", nil, pb.ExonerationReason_UNEXPECTED_PASS),
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
		properties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			},
		}
		propertiesBytes, err := proto.Marshal(properties)
		So(err, ShouldBeNil)

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
				"Properties":      spanutil.Compressed(propertiesBytes),
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
				"SummaryHtml":     spanutil.Compressed("SummaryHtml"),
				"Tags":            pbutil.StringPairsToStrings(strPairs...),
				"TestMetadata":    spanutil.Compressed(tmdBytes),
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
				"30/T5/c467ccce5a16dc72",
				"30/T8/e3b0c44298fc1c14",
				"30/Tx/e3b0c44298fc1c14",
				"40/T1/e3b0c44298fc1c14",
				"40/T2/e3b0c44298fc1c14",
			})

			So(tvs[0].Results, ShouldResembleProto, []*pb.TestResultBundle{
				{
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
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": structpb.NewStringValue("value"),
							},
						},
						Tags: strPairs,
					},
				},
			})
			So(tvs[0].TestMetadata, ShouldResembleProto, tmd)
			sort.Slice(tvs[7].Exonerations, func(i, j int) bool {
				return tvs[7].Exonerations[i].ExplanationHtml < tvs[7].Exonerations[j].ExplanationHtml
			})
			So(len(tvs[7].Exonerations), ShouldEqual, 3)
			So(tvs[7].Exonerations[0], ShouldResembleProto, &pb.TestExoneration{
				ExplanationHtml: "explanation 0",
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			})

			So(tvs[7].Exonerations[1], ShouldResembleProto, &pb.TestExoneration{
				ExplanationHtml: "explanation 1",
				Reason:          pb.ExonerationReason_NOT_CRITICAL,
			})
			So(tvs[7].Exonerations[2], ShouldResembleProto, &pb.TestExoneration{
				ExplanationHtml: "explanation 2",
				Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
			})
			So(len(tvs[8].Exonerations), ShouldEqual, 1)
			So(tvs[8].Exonerations[0], ShouldResembleProto, &pb.TestExoneration{
				ExplanationHtml: "explanation 0",
				Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
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

		Convey(`Field mask works`, func() {
			Convey(`with minimum field mask`, func() {
				verifyFields := func(tvs []*pb.TestVariant) {
					for _, tv := range tvs {
						// Check all results have and only have .TestId, .VariantHash,
						// .Status populated.
						// Those fields should be included even when not specified.
						So(tv.TestId, ShouldNotBeEmpty)
						So(tv.VariantHash, ShouldNotBeEmpty)
						So(tv.Status, ShouldNotBeEmpty)
						So(tv, ShouldResembleProto, &pb.TestVariant{
							TestId:      tv.TestId,
							VariantHash: tv.VariantHash,
							Status:      tv.Status,
						})
					}
				}

				Convey(`with non-expected test variants`, func() {
					q.Mask = mask.MustFromReadMask(
						&pb.TestVariant{},
						"test_id",
					)
					tvs, _ := mustFetch(q)
					verifyFields(tvs)

					// TestId should still be populated even when not specified.
					q.Mask = mask.MustFromReadMask(
						&pb.TestVariant{},
						"status",
					)
					tvs, _ = mustFetch(q)
					verifyFields(tvs)
				})

				Convey(`with expected test variants`, func() {
					// Ensure the last test result (ordered by TestId, then by VariantHash)
					// is expected, so we can verify that the tail is trimmed properly.
					testutil.MustApply(ctx,
						insert.TestResults("inv1", "Tz0", nil, pb.TestStatus_PASS)...,
					)
					q.PageToken = pagination.Token("EXPECTED", "", "")

					q.Mask = mask.MustFromReadMask(
						&pb.TestVariant{},
						"test_id",
					)
					tvs, _ := mustFetch(q)
					verifyFields(tvs)
					So(tvs[len(tvs)-1].TestId, ShouldEqual, "Tz0")

					// TestId should still be populated even when not specified.
					q.Mask = mask.MustFromReadMask(
						&pb.TestVariant{},
						"status",
					)
					tvs, _ = mustFetch(q)
					verifyFields(tvs)
					So(tvs[len(tvs)-1].TestId, ShouldEqual, "Tz0")
				})

			})

			Convey(`with full field mask`, func() {
				q.Mask = mask.MustFromReadMask(
					&pb.TestVariant{},
					"*",
				)

				verifyFields := func(tvs []*pb.TestVariant) {
					for _, tv := range tvs {
						So(tv.TestId, ShouldNotBeEmpty)
						So(tv.VariantHash, ShouldNotBeEmpty)
						So(tv.Status, ShouldNotBeEmpty)
						So(tv.Variant, ShouldNotBeEmpty)
						So(tv.Results, ShouldNotBeEmpty)
						So(tv.TestMetadata, ShouldNotBeEmpty)

						if tv.Status == pb.TestVariantStatus_EXONERATED {
							So(tv.Exonerations, ShouldNotBeEmpty)
						}

						for _, result := range tv.Results {
							So(result.Result.Name, ShouldNotBeEmpty)
							So(result.Result.ResultId, ShouldNotBeEmpty)
							So(result.Result.Expected, ShouldNotBeEmpty)
							So(result.Result.Status, ShouldNotBeEmpty)
							So(result.Result.SummaryHtml, ShouldNotBeBlank)
							So(result.Result.Duration, ShouldNotBeNil)
							So(result.Result.Tags, ShouldNotBeNil)
							if tv.TestId == "T4" {
								So(result.Result.FailureReason, ShouldNotBeNil)
								So(result.Result.Properties, ShouldNotBeNil)
							}
							So(result, ShouldResembleProto, &pb.TestResultBundle{
								Result: &pb.TestResult{
									Name:          result.Result.Name,
									ResultId:      result.Result.ResultId,
									Expected:      result.Result.Expected,
									Status:        result.Result.Status,
									SummaryHtml:   result.Result.SummaryHtml,
									StartTime:     result.Result.StartTime,
									Duration:      result.Result.Duration,
									Tags:          result.Result.Tags,
									FailureReason: result.Result.FailureReason,
									Properties:    result.Result.Properties,
								},
							})
						}

						for _, exoneration := range tv.Exonerations {
							So(exoneration.ExplanationHtml, ShouldNotBeEmpty)
							if tv.TestId != "T2" {
								So(exoneration.Reason, ShouldNotBeZeroValue)
							}
							So(exoneration, ShouldResembleProto, &pb.TestExoneration{
								ExplanationHtml: exoneration.ExplanationHtml,
								Reason:          exoneration.Reason,
							})
						}

						So(tv, ShouldResembleProto, &pb.TestVariant{
							TestId:       tv.TestId,
							VariantHash:  tv.VariantHash,
							Status:       tv.Status,
							Results:      tv.Results,
							Exonerations: tv.Exonerations,
							Variant:      tv.Variant,
							TestMetadata: tv.TestMetadata,
						})
					}
				}

				Convey(`with non-expected test variants`, func() {
					tvs, _ := mustFetch(q)
					verifyFields(tvs)
				})

				Convey(`with expected test variants`, func() {
					// Ensure the last test result (ordered by TestId, then by VariantHash)
					// is expected, so we can verify that the tail is trimmed properly.
					testutil.MustApply(ctx,
						insert.TestResults("inv1", "Tz0", nil, pb.TestStatus_PASS)...,
					)
					q.PageToken = pagination.Token("EXPECTED", "", "")
					tvs, _ := mustFetch(q)
					verifyFields(tvs)
					So(tvs[len(tvs)-1].TestId, ShouldEqual, "Tz0")
				})
			})
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
				"30/T5/c467ccce5a16dc72",
				"30/T8/e3b0c44298fc1c14",
				"30/Tx/e3b0c44298fc1c14",
				"40/T1/e3b0c44298fc1c14",
				"40/T2/e3b0c44298fc1c14",
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
			So(nextToken, ShouldEqual, "CghFWFBFQ1RFRAoCVHkKEGUzYjBjNDQyOThmYzFjMTQ=")

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
					"30/T5/c467ccce5a16dc72",
					"30/T8/e3b0c44298fc1c14",
					"30/Tx/e3b0c44298fc1c14",
					"40/T1/e3b0c44298fc1c14",
					"40/T2/e3b0c44298fc1c14",
				})
			})
		})

		Convey(`ResultLimit works`, func() {
			q.ResultLimit = 2
			tvs, _ := mustFetch(q)

			for _, tv := range tvs {
				So(len(tv.Results), ShouldBeLessThanOrEqualTo, q.ResultLimit)
			}
		})

		Convey(`ResponseLimitBytes works`, func() {
			q.ResponseLimitBytes = 1

			var allTVs []*pb.TestVariant
			fetchAll(q, func(tvs []*pb.TestVariant) {
				// Expect at most one test variant per page.
				So(len(tvs), ShouldBeLessThanOrEqualTo, 1)
				allTVs = append(allTVs, tvs...)
			})

			// All test variants should be returned.
			So(getTVStrings(allTVs), ShouldResemble, []string{
				"10/T4/c467ccce5a16dc72",
				"10/T5/e3b0c44298fc1c14",
				"10/Ty/e3b0c44298fc1c14",
				"20/Tz/e3b0c44298fc1c14",
				"30/T5/c467ccce5a16dc72",
				"30/T8/e3b0c44298fc1c14",
				"30/Tx/e3b0c44298fc1c14",
				"40/T1/e3b0c44298fc1c14",
				"40/T2/e3b0c44298fc1c14",
				"50/T3/e3b0c44298fc1c14",
				"50/T6/e3b0c44298fc1c14",
				"50/T7/e3b0c44298fc1c14",
				"50/T9/e3b0c44298fc1c14",
			})
		})

		Convey(`PageSize works`, func() {
			q.PageSize = 1

			var allTVs []*pb.TestVariant
			fetchAll(q, func(tvs []*pb.TestVariant) {
				// Expect at most one test variant per page.
				So(len(tvs), ShouldBeLessThanOrEqualTo, 1)
				allTVs = append(allTVs, tvs...)
			})

			// All test variants should be returned.
			So(getTVStrings(allTVs), ShouldResemble, []string{
				"10/T4/c467ccce5a16dc72",
				"10/T5/e3b0c44298fc1c14",
				"10/Ty/e3b0c44298fc1c14",
				"20/Tz/e3b0c44298fc1c14",
				"30/T5/c467ccce5a16dc72",
				"30/T8/e3b0c44298fc1c14",
				"30/Tx/e3b0c44298fc1c14",
				"40/T1/e3b0c44298fc1c14",
				"40/T2/e3b0c44298fc1c14",
				"50/T3/e3b0c44298fc1c14",
				"50/T6/e3b0c44298fc1c14",
				"50/T7/e3b0c44298fc1c14",
				"50/T9/e3b0c44298fc1c14",
			})
		})

		Convey(`Empty Invocation works`, func() {
			reachableInvs := make(graph.ReachableInvocations)
			reachableInvs["invnotexists"] = graph.ReachableInvocation{}
			q.ReachableInvocations = reachableInvs

			var allTVs []*pb.TestVariant
			fetchAll(q, func(tvs []*pb.TestVariant) {
				allTVs = append(allTVs, tvs...)
			})
			So(allTVs, ShouldHaveLength, 0)
		})
	})
}

func TestAdjustResultLimit(t *testing.T) {
	t.Parallel()
	Convey(`AdjustResultLimit`, t, func() {
		Convey(`OK`, func() {
			So(AdjustResultLimit(50), ShouldEqual, 50)
		})
		Convey(`Too big`, func() {
			So(AdjustResultLimit(1e6), ShouldEqual, resultLimitMax)
		})
		Convey(`Missing or 0`, func() {
			So(AdjustResultLimit(0), ShouldEqual, resultLimitDefault)
		})
	})
}
