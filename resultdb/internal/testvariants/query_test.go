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
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"
)

func TestQueryTestVariants(t *testing.T) {
	Convey(`QueryTestVariants`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testresultrealm", Permission: rdbperms.PermGetTestResult},
				{Realm: "testproject:testexonerationrealm", Permission: rdbperms.PermGetTestExoneration},
			},
		})

		sources := testutil.TestSourcesWithChangelistNumbers(1)

		reachableInvs := graph.NewReachableInvocations()
		reachableInvs.Invocations["inv0"] = graph.ReachableInvocation{
			HasTestResults:      true,
			HasTestExonerations: true,
			Instructions: &pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id:   "test",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
					},
				},
			},
		}
		reachableInvs.Invocations["inv1"] = graph.ReachableInvocation{
			HasTestResults: true,
			SourceHash:     graph.HashSources(sources),
			Instructions: &pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id:   "test",
						Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						InstructionFilter: &pb.InstructionFilter{
							FilterType: &pb.InstructionFilter_InvocationIds{
								InvocationIds: &pb.InstructionFilterByInvocationID{
									InvocationIds: []string{"inv3"},
								},
							},
						},
					},
				},
			},
			IncludedInvocationIDs: []invocations.ID{"inv3"},
		}
		reachableInvs.Invocations["inv2"] = graph.ReachableInvocation{
			HasTestExonerations: true,
		}
		reachableInvs.Invocations["inv3"] = graph.ReachableInvocation{
			HasTestResults: true,
		}

		reachableInvs.Sources[graph.HashSources(sources)] = sources

		q := &Query{
			ReachableInvocations: reachableInvs,
			PageSize:             100,
			ResultLimit:          10,
			ResponseLimitBytes:   DefaultResponseLimitBytes,
			AccessLevel:          AccessLevelUnrestricted,
		}

		fetch := func(q *Query) (Page, error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return q.Fetch(ctx)
		}

		mustFetch := func(q *Query) Page {
			page, err := fetch(q)
			So(err, ShouldBeNil)
			return page
		}

		fetchAll := func(q *Query, f func(page Page)) {
			for {
				page, err := fetch(q)
				So(err, ShouldBeNil)

				f(page)
				if page.NextPageToken == "" {
					break
				}

				// The page token should always advance, it should
				// never remain the same.
				So(page.NextPageToken, ShouldNotEqual, q.PageToken)
				q.PageToken = page.NextPageToken
			}
		}

		tvStrings := func(tvs []*pb.TestVariant) []string {
			keys := make([]string, len(tvs))
			for i, tv := range tvs {
				instructionName := tv.GetInstruction().GetInstruction()
				keys[i] = fmt.Sprintf("%d/%s/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash, instructionName)
			}
			return keys
		}

		testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, insert.Invocation("inv1", pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, insert.Invocation("inv2", pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, insert.Invocation("inv3", pb.Invocation_ACTIVE, nil))
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
			insert.TestResults("inv3", "Tw", nil, pb.TestStatus_PASS),
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
			},
			BugComponent: &pb.BugComponent{
				System: &pb.BugComponent_Monorail{
					Monorail: &pb.MonorailComponent{
						Project: "chromium",
						Value:   "Component>Value",
					},
				},
			},
		}
		tmdBytes, _ := proto.Marshal(tmd)

		failureReason := &pb.FailureReason{
			PrimaryErrorMessage: "primary error msg",
			Errors: []*pb.FailureReason_Error{
				{Message: "primary error msg"},
				{Message: "primary error msg2"},
			},
			TruncatedErrorsCount: 0,
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
			spanutil.InsertMap("TestResults", map[string]any{
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
			spanutil.InsertMap("TestResults", map[string]any{
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
				"SkipReason":      pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
			}),
		)

		Convey(`Unexpected works`, func() {
			page := mustFetch(q)
			tvs := page.TestVariants
			tvStrings := tvStrings(tvs)
			So(tvStrings, ShouldResemble, []string{
				"10/T4/c467ccce5a16dc72/",
				"10/T5/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"10/Ty/e3b0c44298fc1c14/",
				"20/Tz/e3b0c44298fc1c14/",
				"30/T5/c467ccce5a16dc72/",
				"30/T8/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"30/Tx/e3b0c44298fc1c14/",
				"40/T1/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"40/T2/e3b0c44298fc1c14/invocations/inv0/instructions/test",
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
							Errors: []*pb.FailureReason_Error{
								{Message: "primary error msg"},
								{Message: "primary error msg2"},
							},
							TruncatedErrorsCount: 0,
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
			So(tvs[0].SourcesId, ShouldEqual, graph.HashSources(sources).String())

			sort.Slice(tvs[7].Exonerations, func(i, j int) bool {
				return tvs[7].Exonerations[i].ExplanationHtml < tvs[7].Exonerations[j].ExplanationHtml
			})
			So(len(tvs[7].Exonerations), ShouldEqual, 3)
			So(tvs[7].Exonerations[0], ShouldResembleProto, &pb.TestExoneration{
				Name:            "invocations/inv0/tests/T1/exonerations/0",
				ExplanationHtml: "explanation 0",
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			})

			So(tvs[7].Exonerations[1], ShouldResembleProto, &pb.TestExoneration{
				Name:            "invocations/inv0/tests/T1/exonerations/1",
				ExplanationHtml: "explanation 1",
				Reason:          pb.ExonerationReason_NOT_CRITICAL,
			})
			So(tvs[7].Exonerations[2], ShouldResembleProto, &pb.TestExoneration{
				Name:            "invocations/inv0/tests/T1/exonerations/2",
				ExplanationHtml: "explanation 2",
				Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
			})
			So(len(tvs[8].Exonerations), ShouldEqual, 1)
			So(tvs[8].Exonerations[0], ShouldResembleProto, &pb.TestExoneration{
				Name:            "invocations/inv2/tests/T2/exonerations/0",
				ExplanationHtml: "explanation 0",
				Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
			})
			So(len(tvs[2].Results), ShouldEqual, 10)

			So(page.DistinctSources, ShouldHaveLength, 1)
			So(page.DistinctSources[graph.HashSources(sources).String()], ShouldResembleProto, sources)
		})

		Convey(`Expected works`, func() {
			q.PageToken = pagination.Token("EXPECTED", "", "")
			page := mustFetch(q)
			tvs := page.TestVariants
			So(tvStrings(tvs), ShouldResemble, []string{
				"50/T3/e3b0c44298fc1c14/",
				"50/T6/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/T7/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/T9/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/Tw/e3b0c44298fc1c14/invocations/inv1/instructions/test",
			})
			So(len(tvs[0].Results), ShouldEqual, 2)

			So(tvs[0].SourcesId, ShouldEqual, graph.HashSources(sources).String()) // Sources from inv1.
			So(tvs[1].SourcesId, ShouldBeEmpty)                                    // Sources from inv0.
			So(tvs[2].SourcesId, ShouldBeEmpty)                                    // Sources from inv0.
			So(tvs[3].SourcesId, ShouldBeEmpty)                                    // Sources from inv0.

			So(page.DistinctSources, ShouldHaveLength, 1)
			So(page.DistinctSources[graph.HashSources(sources).String()], ShouldResembleProto, sources)
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
					page := mustFetch(q)
					tvs := page.TestVariants
					verifyFields(tvs)

					// TestId should still be populated even when not specified.
					q.Mask = mask.MustFromReadMask(
						&pb.TestVariant{},
						"status",
					)
					page = mustFetch(q)
					tvs = page.TestVariants
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
					page := mustFetch(q)
					tvs := page.TestVariants
					verifyFields(tvs)
					So(tvs[len(tvs)-1].TestId, ShouldEqual, "Tz0")

					// TestId should still be populated even when not specified.
					q.Mask = mask.MustFromReadMask(
						&pb.TestVariant{},
						"status",
					)
					page = mustFetch(q)
					tvs = page.TestVariants
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
						if tv.TestId == "T3" {
							So(tv.SourcesId, ShouldNotBeEmpty)
						}

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
							So(result.Result.SkipReason, ShouldNotBeEmpty)
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
									SkipReason:    result.Result.SkipReason,
								},
							})
						}

						for _, exoneration := range tv.Exonerations {
							So(exoneration.ExplanationHtml, ShouldNotBeEmpty)
							if tv.TestId != "T2" {
								So(exoneration.Reason, ShouldNotBeZeroValue)
							}
							So(exoneration, ShouldResembleProto, &pb.TestExoneration{
								Name:            exoneration.Name,
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
							SourcesId:    tv.SourcesId,
							Instruction:  tv.Instruction,
						})
					}
				}

				Convey(`with non-expected test variants`, func() {
					page := mustFetch(q)
					tvs := page.TestVariants
					verifyFields(tvs)
				})

				Convey(`with expected test variants`, func() {
					// Ensure the last test result (ordered by TestId, then by VariantHash)
					// is expected, so we can verify that the tail is trimmed properly.
					testutil.MustApply(ctx,
						insert.TestResults("inv1", "Tz0", nil, pb.TestStatus_PASS)...,
					)
					q.PageToken = pagination.Token("EXPECTED", "", "")
					page := mustFetch(q)
					tvs := page.TestVariants
					verifyFields(tvs)
					So(tvs[len(tvs)-1].TestId, ShouldEqual, "Tz0")
				})
			})
		})

		Convey(`paging works`, func() {
			page := func(token string, expectedTVLen int32, expectedTVStrings []string) string {
				q.PageToken = token
				page := mustFetch(q)
				So(tvStrings(page.TestVariants), ShouldResemble, expectedTVStrings)
				return page.NextPageToken
			}

			q.PageSize = 15
			nextToken := page("", 8, []string{
				"10/T4/c467ccce5a16dc72/",
				"10/T5/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"10/Ty/e3b0c44298fc1c14/",
				"20/Tz/e3b0c44298fc1c14/",
				"30/T5/c467ccce5a16dc72/",
				"30/T8/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"30/Tx/e3b0c44298fc1c14/",
				"40/T1/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"40/T2/e3b0c44298fc1c14/invocations/inv0/instructions/test",
			})
			So(nextToken, ShouldEqual, pagination.Token("EXPECTED", "", ""))

			nextToken = page(nextToken, 1, []string{
				"50/T3/e3b0c44298fc1c14/",
			})
			So(nextToken, ShouldEqual, pagination.Token("EXPECTED", "T5", "e3b0c44298fc1c14"))

			nextToken = page(nextToken, 2, []string{
				"50/T6/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/T7/e3b0c44298fc1c14/invocations/inv0/instructions/test",
			})
			So(nextToken, ShouldEqual, pagination.Token("EXPECTED", "T8", "e3b0c44298fc1c14"))

			nextToken = page(nextToken, 2, []string{"50/T9/e3b0c44298fc1c14/invocations/inv0/instructions/test", "50/Tw/e3b0c44298fc1c14/invocations/inv1/instructions/test"})
			So(nextToken, ShouldEqual, "CghFWFBFQ1RFRAoCVHkKEGUzYjBjNDQyOThmYzFjMTQ=")

			nextToken = page(nextToken, 0, []string{})
			So(nextToken, ShouldEqual, "")
		})

		Convey(`Page Token`, func() {
			Convey(`wrong number of parts`, func() {
				q.PageToken = pagination.Token("testId", "variantHash")
				_, err := q.Fetch(ctx)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
			})

			Convey(`first part not tvStatus`, func() {
				q.PageToken = pagination.Token("50", "testId", "variantHash")
				_, err := q.Fetch(ctx)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
			})
		})

		Convey(`status filter works`, func() {
			Convey(`only unexpected`, func() {
				q.Predicate = &pb.TestVariantPredicate{Status: pb.TestVariantStatus_UNEXPECTED}
				page := mustFetch(q)
				tvStrings := tvStrings(page.TestVariants)
				So(tvStrings, ShouldResemble, []string{
					"10/T4/c467ccce5a16dc72/",
					"10/T5/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"10/Ty/e3b0c44298fc1c14/",
				})
				So(page.NextPageToken, ShouldEqual, "")
			})

			Convey(`only expected`, func() {
				q.Predicate = &pb.TestVariantPredicate{Status: pb.TestVariantStatus_EXPECTED}
				page := mustFetch(q)
				So(tvStrings(page.TestVariants), ShouldResemble, []string{
					"50/T3/e3b0c44298fc1c14/",
					"50/T6/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"50/T7/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"50/T9/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"50/Tw/e3b0c44298fc1c14/invocations/inv1/instructions/test",
				})
				So(len(page.TestVariants[0].Results), ShouldEqual, 2)
			})

			Convey(`any unexpected or exonerated`, func() {
				q.Predicate = &pb.TestVariantPredicate{Status: pb.TestVariantStatus_UNEXPECTED_MASK}
				page := mustFetch(q)
				So(tvStrings(page.TestVariants), ShouldResemble, []string{
					"10/T4/c467ccce5a16dc72/",
					"10/T5/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"10/Ty/e3b0c44298fc1c14/",
					"20/Tz/e3b0c44298fc1c14/",
					"30/T5/c467ccce5a16dc72/",
					"30/T8/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"30/Tx/e3b0c44298fc1c14/",
					"40/T1/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"40/T2/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				})
			})
		})

		Convey(`ResultLimit works`, func() {
			q.ResultLimit = 2
			page := mustFetch(q)

			for _, tv := range page.TestVariants {
				So(len(tv.Results), ShouldBeLessThanOrEqualTo, q.ResultLimit)
			}
		})

		Convey(`ResponseLimitBytes works`, func() {
			q.ResponseLimitBytes = 1

			var allTVs []*pb.TestVariant
			fetchAll(q, func(page Page) {
				// Expect at most one test variant per page.
				So(len(page.TestVariants), ShouldBeLessThanOrEqualTo, 1)
				allTVs = append(allTVs, page.TestVariants...)
			})

			// All test variants should be returned.
			So(tvStrings(allTVs), ShouldResemble, []string{
				"10/T4/c467ccce5a16dc72/",
				"10/T5/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"10/Ty/e3b0c44298fc1c14/",
				"20/Tz/e3b0c44298fc1c14/",
				"30/T5/c467ccce5a16dc72/",
				"30/T8/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"30/Tx/e3b0c44298fc1c14/",
				"40/T1/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"40/T2/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/T3/e3b0c44298fc1c14/",
				"50/T6/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/T7/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/T9/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/Tw/e3b0c44298fc1c14/invocations/inv1/instructions/test",
			})
		})

		Convey(`PageSize works`, func() {
			q.PageSize = 1

			var allTVs []*pb.TestVariant
			fetchAll(q, func(page Page) {
				// Expect at most one test variant per page.
				So(len(page.TestVariants), ShouldBeLessThanOrEqualTo, 1)
				allTVs = append(allTVs, page.TestVariants...)
			})

			// All test variants should be returned.
			So(tvStrings(allTVs), ShouldResemble, []string{
				"10/T4/c467ccce5a16dc72/",
				"10/T5/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"10/Ty/e3b0c44298fc1c14/",
				"20/Tz/e3b0c44298fc1c14/",
				"30/T5/c467ccce5a16dc72/",
				"30/T8/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"30/Tx/e3b0c44298fc1c14/",
				"40/T1/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"40/T2/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/T3/e3b0c44298fc1c14/",
				"50/T6/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/T7/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/T9/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				"50/Tw/e3b0c44298fc1c14/invocations/inv1/instructions/test",
			})
		})

		Convey(`Querying with limited access works`, func() {
			q.AccessLevel = AccessLevelLimited

			Convey(`with only limited access`, func() {
				page := mustFetch(q)
				So(tvStrings(page.TestVariants), ShouldResemble, []string{
					"10/T4/c467ccce5a16dc72/",
					"10/T5/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"10/Ty/e3b0c44298fc1c14/",
					"20/Tz/e3b0c44298fc1c14/",
					"30/T5/c467ccce5a16dc72/",
					"30/T8/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"30/Tx/e3b0c44298fc1c14/",
					"40/T1/e3b0c44298fc1c14/invocations/inv0/instructions/test",
					"40/T2/e3b0c44298fc1c14/invocations/inv0/instructions/test",
				})

				// Check the test variant and its test results and test exonerations
				// have all been masked.
				for _, tv := range page.TestVariants {
					So(tv.TestMetadata, ShouldBeNil)
					So(tv.Variant, ShouldBeNil)
					So(tv.IsMasked, ShouldBeTrue)

					for _, tr := range tv.Results {
						So(tr.Result.IsMasked, ShouldBeTrue)
					}

					for _, te := range tv.Exonerations {
						So(te.IsMasked, ShouldBeTrue)
					}
				}

				// Check test results and exonerations for data that should
				// still be available after masking to limited data.
				So(page.TestVariants[0].Results, ShouldResembleProto, []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:      "invocations/inv1/tests/T4/results/0",
							ResultId:  "0",
							Expected:  false,
							Status:    pb.TestStatus_FAIL,
							StartTime: startTime,
							Duration:  duration,
							FailureReason: &pb.FailureReason{
								PrimaryErrorMessage: "primary error msg",
								Errors: []*pb.FailureReason_Error{
									{Message: "primary error msg"},
									{Message: "primary error msg2"},
								},
								TruncatedErrorsCount: 0,
							},
							IsMasked: true,
						},
					},
				})
				So(len(page.TestVariants[8].Exonerations), ShouldEqual, 1)
				So(page.TestVariants[8].Exonerations[0], ShouldResembleProto, &pb.TestExoneration{
					Name:            "invocations/inv2/tests/T2/exonerations/0",
					ExplanationHtml: "explanation 0",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
					IsMasked:        true,
				})
				So(len(page.TestVariants[2].Results), ShouldEqual, 10)
			})

			Convey(`with unrestricted test result access`, func() {
				reachableInvs := graph.NewReachableInvocations()
				reachableInvs.Invocations["inv0"] = graph.ReachableInvocation{
					HasTestResults:      true,
					HasTestExonerations: true,
					Realm:               insert.TestRealm,
				}
				reachableInvs.Invocations["inv1"] = graph.ReachableInvocation{
					HasTestResults: true,
					Realm:          "testproject:testresultrealm",
				}
				reachableInvs.Invocations["inv2"] = graph.ReachableInvocation{
					HasTestExonerations: true,
					Realm:               insert.TestRealm,
				}
				q.ReachableInvocations = reachableInvs

				page := mustFetch(q)
				tvs := page.TestVariants
				So(tvStrings(tvs), ShouldResemble, []string{
					"10/T4/c467ccce5a16dc72/",
					"10/T5/e3b0c44298fc1c14/",
					"10/Ty/e3b0c44298fc1c14/",
					"20/Tz/e3b0c44298fc1c14/",
					"30/T5/c467ccce5a16dc72/",
					"30/T8/e3b0c44298fc1c14/",
					"30/Tx/e3b0c44298fc1c14/",
					"40/T1/e3b0c44298fc1c14/",
					"40/T2/e3b0c44298fc1c14/",
				})

				// Check all the test exonerations have been masked.
				for _, tv := range tvs {
					for _, te := range tv.Exonerations {
						So(te.IsMasked, ShouldBeTrue)
					}
				}

				// The only test result for the below test variant should be unmasked
				// as the user has access to unrestricted test results in inv1's realm.
				// Thus, the test variant should also be unmasked.
				So(tvs[0].IsMasked, ShouldBeFalse)
				So(tvs[0].TestMetadata, ShouldResembleProto, tmd)
				So(tvs[0].Variant, ShouldNotBeNil)
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
								Errors: []*pb.FailureReason_Error{
									{Message: "primary error msg"},
									{Message: "primary error msg2"},
								},
								TruncatedErrorsCount: 0,
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

				// Both test results for the below test variant should have been masked.
				// Thus, the test variant should be masked.
				So(tvs[5].IsMasked, ShouldBeTrue)
				So(tvs[5].TestMetadata, ShouldBeNil)
				So(tvs[5].Variant, ShouldBeNil)
				So(len(tvs[5].Results), ShouldEqual, 2)
				for _, tr := range tvs[5].Results {
					So(tr.Result.IsMasked, ShouldBeTrue)
				}

				// The below test variant should have both a masked and
				// unmasked test result due to the different invocations' realms.
				// Thus, the test variant should be unmasked.
				So(tvs[8].IsMasked, ShouldBeFalse)
				So(tvs[8].Results, ShouldResembleProto, []*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:     "invocations/inv0/tests/T2/results/0",
							ResultId: "0",
							Expected: true,
							Status:   pb.TestStatus_PASS,
							Duration: duration,
							IsMasked: true,
						},
					},
					{
						Result: &pb.TestResult{
							Name:     "invocations/inv1/tests/T2/results/0",
							ResultId: "0",
							Expected: false,
							Status:   pb.TestStatus_FAIL,
							FailureReason: &pb.FailureReason{
								PrimaryErrorMessage: "failure reason",
							},
							Duration:    duration,
							SummaryHtml: "SummaryHtml",
							Properties: &structpb.Struct{Fields: map[string]*structpb.Value{
								"key": structpb.NewStringValue("value"),
							}},
						},
					},
				})
				So(len(tvs[8].Exonerations), ShouldEqual, 1)
				So(tvs[8].Exonerations[0], ShouldResembleProto, &pb.TestExoneration{
					Name:            "invocations/inv2/tests/T2/exonerations/0",
					ExplanationHtml: "explanation 0",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
					IsMasked:        true,
				})
			})
		})

		Convey(`Empty Invocation works`, func() {
			reachableInvs := graph.NewReachableInvocations()
			reachableInvs.Invocations["invnotexists"] = graph.ReachableInvocation{}
			q.ReachableInvocations = reachableInvs

			var allTVs []*pb.TestVariant
			fetchAll(q, func(page Page) {
				allTVs = append(allTVs, page.TestVariants...)
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
