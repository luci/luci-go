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

package testresults

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	durpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/mask"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

func TestQueryTestResults(t *testing.T) {
	Convey(`QueryTestResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, insert.Invocation("inv1", pb.Invocation_ACTIVE, map[string]any{
			"CommonTestIDPrefix":     "",
			"TestResultVariantUnion": []string{"a:b", "k:1", "k:2", "k2:1"},
		}))

		q := &Query{
			Predicate:     &pb.TestResultPredicate{},
			PageSize:      100,
			InvocationIDs: invocations.NewIDSet("inv1"),
			Mask:          AllFields,
		}

		fetch := func(q *Query) (trs []*pb.TestResult, token string, err error) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			return q.Fetch(ctx)
		}

		mustFetch := func(q *Query) (trs []*pb.TestResult, token string) {
			trs, token, err := fetch(q)
			So(err, ShouldBeNil)
			return
		}

		mustFetchNames := func(q *Query) []string {
			trs, _, err := fetch(q)
			So(err, ShouldBeNil)
			names := make([]string, len(trs))
			for i, a := range trs {
				names[i] = a.Name
			}
			sort.Strings(names)
			return names
		}

		Convey(`Does not fetch test results of other invocations`, func() {
			expected := insert.MakeTestResults("inv1", "DoBaz", nil,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
				pb.TestStatus_FAIL,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
			)
			testutil.MustApply(ctx,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, nil),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, nil),
			)
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "X", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResultMessages(expected),
				insert.TestResults("inv2", "Y", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
			)...)

			actual, _ := mustFetch(q)
			So(actual, ShouldResembleProto, expected)
		})

		Convey(`Expectancy filter`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
				"CommonTestIDPrefix":     "",
				"TestResultVariantUnion": pbutil.Variant("a", "b"),
			}))
			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")

			Convey(`VARIANTS_WITH_UNEXPECTED_RESULTS`, func() {
				q.Predicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS

				testutil.MustApply(ctx, testutil.CombineMutations(
					insert.TestResults("inv0", "T1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
					insert.TestResults("inv0", "T2", nil, pb.TestStatus_PASS),
					insert.TestResults("inv1", "T1", nil, pb.TestStatus_PASS),
					insert.TestResults("inv1", "T2", nil, pb.TestStatus_FAIL),
					insert.TestResults("inv1", "T3", nil, pb.TestStatus_PASS),
					insert.TestResults("inv1", "T4", pbutil.Variant("a", "b"), pb.TestStatus_FAIL),
					insert.TestExonerations("inv0", "T1", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				)...)

				Convey(`Works`, func() {
					So(mustFetchNames(q), ShouldResemble, []string{
						"invocations/inv0/tests/T1/results/0",
						"invocations/inv0/tests/T1/results/1",
						"invocations/inv0/tests/T2/results/0",
						"invocations/inv1/tests/T1/results/0",
						"invocations/inv1/tests/T2/results/0",
						"invocations/inv1/tests/T4/results/0",
					})
				})

				Convey(`TestID filter`, func() {
					q.Predicate.TestIdRegexp = ".*T4"
					So(mustFetchNames(q), ShouldResemble, []string{
						"invocations/inv1/tests/T4/results/0",
					})
				})

				Convey(`Variant filter`, func() {
					q.Predicate.Variant = &pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Equals{
							Equals: pbutil.Variant("a", "b"),
						},
					}
					So(mustFetchNames(q), ShouldResemble, []string{
						"invocations/inv1/tests/T4/results/0",
					})
				})

				Convey(`ExcludeExonerated`, func() {
					q.Predicate.ExcludeExonerated = true
					So(mustFetchNames(q), ShouldResemble, []string{
						"invocations/inv0/tests/T2/results/0",
						"invocations/inv1/tests/T2/results/0",
						"invocations/inv1/tests/T4/results/0",
					})
				})
			})

			Convey(`VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS`, func() {
				q.Predicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS

				testutil.MustApply(ctx, testutil.CombineMutations(
					insert.TestResults("inv0", "flaky", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
					insert.TestResults("inv0", "passing", nil, pb.TestStatus_PASS),
					insert.TestResults("inv0", "F0", pbutil.Variant("a", "0"), pb.TestStatus_FAIL),
					insert.TestResults("inv0", "in_both_invocations", nil, pb.TestStatus_FAIL),
					// Same test, but different variant.
					insert.TestResults("inv1", "F0", pbutil.Variant("a", "1"), pb.TestStatus_PASS),
					insert.TestResults("inv1", "in_both_invocations", nil, pb.TestStatus_PASS),
					insert.TestResults("inv1", "F1", nil, pb.TestStatus_FAIL, pb.TestStatus_FAIL),
				)...)

				Convey(`Works`, func() {
					So(mustFetchNames(q), ShouldResemble, []string{
						"invocations/inv0/tests/F0/results/0",
						"invocations/inv1/tests/F1/results/0",
						"invocations/inv1/tests/F1/results/1",
					})
				})
			})
		})

		Convey(`Test id filter`, func() {
			testutil.MustApply(ctx,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
					"CommonTestIDPrefix": "1-",
				}),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, map[string]any{
					"CreateTime": time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				}),
			)
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "1-1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv0", "1-2", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "1-1", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2-1", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2", nil, pb.TestStatus_FAIL),
				insert.TestResults("inv2", "1-2", nil, pb.TestStatus_PASS),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1", "inv2")
			q.Predicate.TestIdRegexp = "1-.+"

			So(mustFetchNames(q), ShouldResemble, []string{
				"invocations/inv0/tests/1-1/results/0",
				"invocations/inv0/tests/1-1/results/1",
				"invocations/inv0/tests/1-2/results/0",
				"invocations/inv1/tests/1-1/results/0",
				"invocations/inv2/tests/1-2/results/0",
			})
		})

		Convey(`Variant equals`, func() {
			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
					"TestResultVariantUnion": []string{"k:1", "k:2"},
				}),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, map[string]any{
					"CreateTime": time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				}),
			)
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "1-1", v1, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv0", "1-2", v2, pb.TestStatus_PASS),
				insert.TestResults("inv1", "1-1", v1, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2-1", v2, pb.TestStatus_PASS),
				insert.TestResults("inv2", "1-1", v1, pb.TestStatus_PASS),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1", "inv2")
			q.Predicate.Variant = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{Equals: v1},
			}

			So(mustFetchNames(q), ShouldResemble, []string{
				"invocations/inv0/tests/1-1/results/0",
				"invocations/inv0/tests/1-1/results/1",
				"invocations/inv1/tests/1-1/results/0",
				"invocations/inv2/tests/1-1/results/0",
			})
		})

		Convey(`Variant contains`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
				"TestResultVariantUnion": []string{"k:1", "k:2", "k2:1"},
			}))

			v1 := pbutil.Variant("k", "1")
			v11 := pbutil.Variant("k", "1", "k2", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "1-1", v1, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv0", "1-2", v11, pb.TestStatus_PASS),
				insert.TestResults("inv1", "1-1", v1, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2-1", v2, pb.TestStatus_PASS),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")

			Convey(`Empty`, func() {
				q.Predicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: pbutil.Variant()},
				}

				So(mustFetchNames(q), ShouldResemble, []string{
					"invocations/inv0/tests/1-1/results/0",
					"invocations/inv0/tests/1-1/results/1",
					"invocations/inv0/tests/1-2/results/0",
					"invocations/inv1/tests/1-1/results/0",
					"invocations/inv1/tests/2-1/results/0",
				})
			})

			Convey(`Non-empty`, func() {
				q.Predicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: v1},
				}

				So(mustFetchNames(q), ShouldResemble, []string{
					"invocations/inv0/tests/1-1/results/0",
					"invocations/inv0/tests/1-1/results/1",
					"invocations/inv0/tests/1-2/results/0",
					"invocations/inv1/tests/1-1/results/0",
				})
			})
		})

		Convey(`Paging`, func() {
			trs := insert.MakeTestResults("inv1", "DoBaz", nil,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
				pb.TestStatus_FAIL,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
			)
			testutil.MustApply(ctx, insert.TestResultMessages(trs)...)

			mustReadPage := func(pageToken string, pageSize int, expected []*pb.TestResult) string {
				q2 := q
				q2.PageToken = pageToken
				q2.PageSize = pageSize
				actual, token := mustFetch(q2)
				So(actual, ShouldResembleProto, expected)
				return token
			}

			Convey(`All results`, func() {
				token := mustReadPage("", 10, trs)
				So(token, ShouldEqual, "")
			})

			Convey(`With pagination`, func() {
				token := mustReadPage("", 1, trs[:1])
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 4, trs[1:])
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 5, nil)
				So(token, ShouldEqual, "")
			})

			Convey(`Bad token`, func() {
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()

				Convey(`From bad position`, func() {
					q.PageToken = "CgVoZWxsbw=="
					_, _, err := q.Fetch(ctx)
					So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
				})

				Convey(`From decoding`, func() {
					q.PageToken = "%%%"
					_, _, err := q.Fetch(ctx)
					So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
				})
			})
		})

		Convey(`Test metadata`, func() {
			expected := insert.MakeTestResults("inv1", "DoBaz", nil, pb.TestStatus_PASS)
			expected[0].TestMetadata = &pb.TestMetadata{
				Name: "original_name",
				Location: &pb.TestLocation{
					FileName: "//a_test.go",
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
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, insert.TestResultMessages(expected)...)

			actual, _ := mustFetch(q)
			So(actual, ShouldResembleProto, expected)
		})

		Convey(`Failure reason`, func() {
			expected := insert.MakeTestResults("inv1", "DoFailureReason", nil, pb.TestStatus_PASS)
			expected[0].FailureReason = &pb.FailureReason{
				PrimaryErrorMessage: "want true, got false",
				Errors: []*pb.FailureReason_Error{
					{Message: "want true, got false"},
					{Message: "want false, got true"},
				},
				TruncatedErrorsCount: 0,
			}
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, insert.TestResultMessages(expected)...)

			actual, _ := mustFetch(q)
			So(actual, ShouldResembleProto, expected)
		})

		Convey(`Properties`, func() {
			expected := insert.MakeTestResults("inv1", "WithProperties", nil, pb.TestStatus_PASS)
			expected[0].Properties = &structpb.Struct{Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			}}
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, insert.TestResultMessages(expected)...)

			actual, _ := mustFetch(q)
			So(actual, ShouldResembleProto, expected)
		})

		Convey(`Skip reason`, func() {
			expected := insert.MakeTestResults("inv1", "WithSkipReason", nil, pb.TestStatus_SKIP)
			expected[0].SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
			testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, insert.TestResultMessages(expected)...)

			actual, _ := mustFetch(q)
			So(actual, ShouldResembleProto, expected)
		})

		Convey(`Variant in the mask`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))

			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "1-1", v1, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv0", "1-2", v2, pb.TestStatus_PASS),
				insert.TestResults("inv1", "1-1", v1, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2-1", v2, pb.TestStatus_PASS),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")

			Convey(`Present`, func() {
				q.Mask = mask.MustFromReadMask(&pb.TestResult{}, "name", "test_id", "variant", "variant_hash")
				results, _ := mustFetch(q)
				for _, r := range results {
					So(r.Variant, ShouldNotBeNil)
					So(r.VariantHash, ShouldNotEqual, "")
				}
			})

			Convey(`Not present`, func() {
				q.Mask = mask.MustFromReadMask(&pb.TestResult{}, "name", "test_id")
				results, _ := mustFetch(q)
				for _, r := range results {
					So(r.Variant, ShouldBeNil)
					So(r.VariantHash, ShouldEqual, "")
				}
			})
		})

		Convey(`Empty list of invocations`, func() {
			q.InvocationIDs = invocations.NewIDSet()
			actual, _ := mustFetch(q)
			So(actual, ShouldHaveLength, 0)
		})

		Convey(`Filter invocations`, func() {
			testutil.MustApply(ctx,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
					"CommonTestIDPrefix": "ninja://browser_tests/",
				}),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, nil),
			)

			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "ninja://browser_tests/1-1", v1, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv0", "1-2", v2, pb.TestStatus_PASS),
				insert.TestResults("inv1", "ninja://browser_tests/1-1", v1, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2-1", v2, pb.TestStatus_PASS),
				insert.TestResults("inv2", "ninja://browser_tests/1-1", v1, pb.TestStatus_PASS),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")
			q.Predicate.TestIdRegexp = "ninja://browser_tests/.*"

			results, _ := mustFetch(q)
			for _, r := range results {
				So(r.Name, ShouldNotStartWith, "invocations/inv2")
				So(r.TestId, ShouldEqual, "ninja://browser_tests/1-1")
			}
		})

		Convey(`Query statements`, func() {
			Convey(`only unexpected exclude exonerated`, func() {
				st := q.genStatement("testResults", map[string]any{
					"params": map[string]any{
						"invIDs":            q.InvocationIDs,
						"afterInvocationId": "build-123",
						"afterTestId":       "test",
						"afterResultId":     "result",
					},
					"columns":           "InvocationId, TestId, VariantHash",
					"onlyUnexpected":    true,
					"withUnexpected":    true,
					"excludeExonerated": true,
				})
				expected := `
  				@{USE_ADDITIONAL_PARALLELISM=TRUE}
  				WITH
						invs AS (
							SELECT *
							FROM UNNEST(@invIDs)
							AS InvocationId
						),
						testVariants AS (
							SELECT DISTINCT TestId, VariantHash
							FROM invs
							JOIN TestResults@{FORCE_INDEX=UnexpectedTestResults, spanner_emulator.disable_query_null_filtered_index_check=true} USING(InvocationId)
							WHERE IsUnexpected
						),
						exonerated AS (
							SELECT DISTINCT TestId, VariantHash
							FROM TestExonerations
							WHERE InvocationId IN UNNEST(@invIDs)
						),
						variantsWithUnexpectedResults AS (
							SELECT
								tv.*
							FROM testVariants tv
							LEFT JOIN exonerated USING(TestId, VariantHash)
							WHERE exonerated.TestId IS NULL
						),
  					withUnexpected AS (
  						SELECT InvocationId, TestId, VariantHash
							FROM invs
							JOIN (
								variantsWithUnexpectedResults vur
								JOIN@{FORCE_JOIN_ORDER=TRUE, JOIN_METHOD=HASH_JOIN} TestResults tr USING (TestId, VariantHash)
							) USING(InvocationId)
						) ,
						withOnlyUnexpected AS (
							SELECT ARRAY_AGG(tr) trs
							FROM withUnexpected tr
							GROUP BY TestId, VariantHash
							HAVING LOGICAL_AND(IFNULL(IsUnexpected, false))
						)
  				SELECT tr.*
  				FROM withOnlyUnexpected owu, owu.trs tr
					WHERE true
						AND (
							(InvocationId > @afterInvocationId) OR
							(InvocationId = @afterInvocationId AND TestId > @afterTestId) OR
							(InvocationId = @afterInvocationId AND TestId = @afterTestId AND ResultId > @afterResultId)
						)
					ORDER BY InvocationId, TestId, ResultId
				`
				So(strings.Join(strings.Fields(st.SQL), " "), ShouldEqual, strings.Join(strings.Fields(expected), " "))
			})

			Convey(`with unexpected filter by testID`, func() {
				st := q.genStatement("testResults", map[string]any{
					"params": map[string]any{
						"invIDs":            q.InvocationIDs,
						"testIdRegexp":      "^T4$",
						"afterInvocationId": "build-123",
						"afterTestId":       "test",
						"afterResultId":     "result",
					},
					"columns":           "InvocationId, TestId, VariantHash",
					"onlyUnexpected":    false,
					"withUnexpected":    true,
					"excludeExonerated": false,
				})
				expected := `
					@{USE_ADDITIONAL_PARALLELISM=TRUE}
					WITH
						invs AS (
							SELECT *
							FROM UNNEST(@invIDs)
							AS InvocationId
						),
						variantsWithUnexpectedResults AS (
							SELECT DISTINCT TestId, VariantHash
							FROM invs
							JOIN TestResults@{FORCE_INDEX=UnexpectedTestResults, spanner_emulator.disable_query_null_filtered_index_check=true} USING(InvocationId)
							WHERE IsUnexpected
							AND REGEXP_CONTAINS(TestId, @testIdRegexp)
						),
						withUnexpected AS (
							SELECT InvocationId, TestId, VariantHash
							FROM invs
							JOIN (
								variantsWithUnexpectedResults vur
								JOIN@{FORCE_JOIN_ORDER=TRUE, JOIN_METHOD=HASH_JOIN} TestResults tr USING (TestId, VariantHash)
							) USING(InvocationId)
						)
					SELECT * FROM withUnexpected
					WHERE true
						AND (
							(InvocationId > @afterInvocationId) OR
							(InvocationId = @afterInvocationId AND TestId > @afterTestId) OR
							(InvocationId = @afterInvocationId AND TestId = @afterTestId AND ResultId > @afterResultId)
						)
					ORDER BY InvocationId, TestId, ResultId
					`
				// Compare sql strings ignoring whitespaces.
				So(strings.Join(strings.Fields(st.SQL), " "), ShouldEqual, strings.Join(strings.Fields(expected), " "))
			})
		})
	})
}

func TestToLimitedData(t *testing.T) {
	ctx := context.Background()

	Convey(`ToLimitedData`, t, func() {
		invID := "inv0"
		testID := "FooBar"
		resultID := "123"
		name := pbutil.TestResultName(invID, testID, resultID)
		variant := pbutil.Variant()
		variantHash := pbutil.VariantHash(variant)

		Convey(`masks fields`, func() {
			testResult := &pb.TestResult{
				Name:        name,
				TestId:      testID,
				ResultId:    resultID,
				Variant:     variant,
				Expected:    true,
				Status:      pb.TestStatus_PASS,
				SummaryHtml: "SummaryHtml",
				StartTime:   &timestamppb.Timestamp{Seconds: 1000, Nanos: 1234},
				Duration:    &durpb.Duration{Seconds: int64(123), Nanos: 234567000},
				Tags:        pbutil.StringPairs("k1", "v1", "k2", "v2"),
				VariantHash: variantHash,
				TestMetadata: &pb.TestMetadata{
					Name: "name",
					Location: &pb.TestLocation{
						Repo:     "https://chromium.googlesource.com/chromium/src",
						FileName: "//artifact_dir/a_test.cc",
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
				},
				FailureReason: &pb.FailureReason{
					PrimaryErrorMessage: "an error message",
					Errors: []*pb.FailureReason_Error{
						{Message: "an error message"},
						{Message: "an error message2"},
					},
					TruncatedErrorsCount: 0,
				},
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": structpb.NewStringValue("value"),
					},
				},
				SkipReason: pb.SkipReason_SKIP_REASON_UNSPECIFIED,
			}
			So(pbutil.ValidateTestResult(testclock.TestRecentTimeUTC, testResult), ShouldBeNil)

			expected := &pb.TestResult{
				Name:        name,
				TestId:      testID,
				ResultId:    resultID,
				Expected:    true,
				Status:      pb.TestStatus_PASS,
				StartTime:   &timestamppb.Timestamp{Seconds: 1000, Nanos: 1234},
				Duration:    &durpb.Duration{Seconds: int64(123), Nanos: 234567000},
				VariantHash: variantHash,
				FailureReason: &pb.FailureReason{
					PrimaryErrorMessage: "an error message",
					Errors: []*pb.FailureReason_Error{
						{Message: "an error message"},
						{Message: "an error message2"},
					},
					TruncatedErrorsCount: 0,
				},
				IsMasked:   true,
				SkipReason: pb.SkipReason_SKIP_REASON_UNSPECIFIED,
			}
			So(pbutil.ValidateTestResult(testclock.TestRecentTimeUTC, expected), ShouldBeNil)

			err := ToLimitedData(ctx, testResult)
			So(err, ShouldBeNil)
			So(testResult, ShouldResembleProto, expected)
		})

		Convey(`truncates primary error message`, func() {
			testResult := &pb.TestResult{
				Name:        name,
				TestId:      testID,
				ResultId:    resultID,
				Variant:     variant,
				Expected:    false,
				Status:      pb.TestStatus_FAIL,
				SummaryHtml: "SummaryHtml",
				StartTime:   &timestamppb.Timestamp{Seconds: 1000, Nanos: 1234},
				Duration:    &durpb.Duration{Seconds: int64(123), Nanos: 234567000},
				Tags:        pbutil.StringPairs("k1", "v1", "k2", "v2"),
				VariantHash: variantHash,
				TestMetadata: &pb.TestMetadata{
					Name: "name",
					Location: &pb.TestLocation{
						Repo:     "https://chromium.googlesource.com/chromium/src",
						FileName: "//artifact_dir/a_test.cc",
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
				},
				FailureReason: &pb.FailureReason{
					PrimaryErrorMessage: strings.Repeat("a very long error message", 10),
					Errors: []*pb.FailureReason_Error{
						{
							Message: strings.Repeat("a very long error message",
								10),
						},
						{
							Message: strings.Repeat("a very long error message2",
								10),
						},
					},
					TruncatedErrorsCount: 0,
				},
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": structpb.NewStringValue("value"),
					},
				},
				SkipReason: pb.SkipReason_SKIP_REASON_UNSPECIFIED,
			}
			So(pbutil.ValidateTestResult(testclock.TestRecentTimeUTC, testResult), ShouldBeNil)

			limitedLongErrMsg := strings.Repeat("a very long error message",
				10)[:limitedReasonLength] + "..."
			limitedLongErrMsg2 := strings.Repeat("a very long error message2",
				10)[:limitedReasonLength] + "..."
			expected := &pb.TestResult{
				Name:        name,
				TestId:      testID,
				ResultId:    resultID,
				Expected:    false,
				Status:      pb.TestStatus_FAIL,
				StartTime:   &timestamppb.Timestamp{Seconds: 1000, Nanos: 1234},
				Duration:    &durpb.Duration{Seconds: int64(123), Nanos: 234567000},
				VariantHash: variantHash,
				FailureReason: &pb.FailureReason{
					PrimaryErrorMessage: limitedLongErrMsg,
					Errors: []*pb.FailureReason_Error{
						{Message: limitedLongErrMsg},
						{Message: limitedLongErrMsg2},
					},
					TruncatedErrorsCount: 0,
				},
				IsMasked:   true,
				SkipReason: pb.SkipReason_SKIP_REASON_UNSPECIFIED,
			}
			So(pbutil.ValidateTestResult(testclock.TestRecentTimeUTC, expected), ShouldBeNil)

			err := ToLimitedData(ctx, testResult)
			So(err, ShouldBeNil)
			So(testResult, ShouldResembleProto, expected)
		})
		Convey(`mask preserves skip reason`, func() {
			testResult := &pb.TestResult{
				Name:        name,
				TestId:      testID,
				ResultId:    resultID,
				Variant:     variant,
				Expected:    true,
				Status:      pb.TestStatus_SKIP,
				VariantHash: variantHash,
				SkipReason:  pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
			}
			So(pbutil.ValidateTestResult(testclock.TestRecentTimeUTC, testResult), ShouldBeNil)

			expected := &pb.TestResult{
				Name:        name,
				TestId:      testID,
				ResultId:    resultID,
				Expected:    true,
				Status:      pb.TestStatus_SKIP,
				VariantHash: variantHash,
				IsMasked:    true,
				SkipReason:  pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
			}
			So(pbutil.ValidateTestResult(testclock.TestRecentTimeUTC, expected), ShouldBeNil)

			err := ToLimitedData(ctx, testResult)
			So(err, ShouldBeNil)
			So(testResult, ShouldResembleProto, expected)
		})
	})
}
