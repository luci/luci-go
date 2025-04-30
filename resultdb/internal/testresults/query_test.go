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

	"google.golang.org/grpc/codes"
	durpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQueryTestResults(t *testing.T) {
	ftt.Run(`QueryTestResults`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, t, insert.Invocation("inv1", pb.Invocation_ACTIVE, map[string]any{
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
			assert.Loosely(t, err, should.BeNil)
			return
		}

		mustFetchNames := func(q *Query) []string {
			trs, _, err := fetch(q)
			assert.Loosely(t, err, should.BeNil)
			names := make([]string, len(trs))
			for i, a := range trs {
				names[i] = a.Name
			}
			sort.Strings(names)
			return names
		}

		t.Run(`Does not fetch test results of other invocations`, func(t *ftt.Test) {
			expected := insert.MakeTestResults("inv1", "DoBaz", &pb.Variant{},
				pb.TestResult_PASSED,
				pb.TestResult_FAILED,
				pb.TestResult_FAILED,
				pb.TestResult_PASSED,
				pb.TestResult_FAILED,
			)

			testutil.MustApply(ctx, t,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, nil),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, nil),
			)
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv0", "X", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
				insert.TestResultMessages(t, expected),
				insert.TestResults(t, "inv2", "Y", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
			)...)

			actual, _ := mustFetch(q)
			assert.Loosely(t, actual, should.Match(expected))
		})

		t.Run(`Expectancy filter`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
				"CommonTestIDPrefix":     "",
				"TestResultVariantUnion": pbutil.Variant("a", "b"),
			}))
			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")

			t.Run(`VARIANTS_WITH_UNEXPECTED_RESULTS`, func(t *ftt.Test) {
				q.Predicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS

				testutil.MustApply(ctx, t, testutil.CombineMutations(
					insert.TestResults(t, "inv0", "T1", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
					insert.TestResults(t, "inv0", "T2", nil, pb.TestResult_PASSED),
					insert.TestResults(t, "inv1", "T1", nil, pb.TestResult_PASSED),
					insert.TestResults(t, "inv1", "T2", nil, pb.TestResult_FAILED),
					insert.TestResults(t, "inv1", "T3", nil, pb.TestResult_PASSED),
					insert.TestResults(t, "inv1", "T4", pbutil.Variant("a", "b"), pb.TestResult_FAILED),
					insert.TestExonerations("inv0", "T1", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				)...)

				t.Run(`Works`, func(t *ftt.Test) {
					assert.Loosely(t, mustFetchNames(q), should.Match([]string{
						"invocations/inv0/tests/T1/results/0",
						"invocations/inv0/tests/T1/results/1",
						"invocations/inv0/tests/T2/results/0",
						"invocations/inv1/tests/T1/results/0",
						"invocations/inv1/tests/T2/results/0",
						"invocations/inv1/tests/T4/results/0",
					}))
				})

				t.Run(`TestID filter`, func(t *ftt.Test) {
					q.Predicate.TestIdRegexp = ".*T4"
					assert.Loosely(t, mustFetchNames(q), should.Match([]string{
						"invocations/inv1/tests/T4/results/0",
					}))
				})

				t.Run(`Variant filter`, func(t *ftt.Test) {
					q.Predicate.Variant = &pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Equals{
							Equals: pbutil.Variant("a", "b"),
						},
					}
					assert.Loosely(t, mustFetchNames(q), should.Match([]string{
						"invocations/inv1/tests/T4/results/0",
					}))
				})

				t.Run(`ExcludeExonerated`, func(t *ftt.Test) {
					q.Predicate.ExcludeExonerated = true
					assert.Loosely(t, mustFetchNames(q), should.Match([]string{
						"invocations/inv0/tests/T2/results/0",
						"invocations/inv1/tests/T2/results/0",
						"invocations/inv1/tests/T4/results/0",
					}))
				})
			})

			t.Run(`VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS`, func(t *ftt.Test) {
				q.Predicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_ONLY_UNEXPECTED_RESULTS

				testutil.MustApply(ctx, t, testutil.CombineMutations(
					insert.TestResults(t, "inv0", "flaky", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
					insert.TestResults(t, "inv0", "passing", nil, pb.TestResult_PASSED),
					insert.TestResults(t, "inv0", "F0", pbutil.Variant("a", "0"), pb.TestResult_FAILED),
					insert.TestResults(t, "inv0", "in_both_invocations", nil, pb.TestResult_FAILED),
					// Same test, but different variant.
					insert.TestResults(t, "inv1", "F0", pbutil.Variant("a", "1"), pb.TestResult_PASSED),
					insert.TestResults(t, "inv1", "in_both_invocations", nil, pb.TestResult_PASSED),
					insert.TestResults(t, "inv1", "F1", nil, pb.TestResult_FAILED, pb.TestResult_FAILED),
				)...)

				t.Run(`Works`, func(t *ftt.Test) {
					assert.Loosely(t, mustFetchNames(q), should.Match([]string{
						"invocations/inv0/tests/F0/results/0",
						"invocations/inv1/tests/F1/results/0",
						"invocations/inv1/tests/F1/results/1",
					}))
				})
			})
		})

		t.Run(`Test id filter`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
					"CommonTestIDPrefix": "1-",
				}),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, map[string]any{
					"CreateTime": time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				}),
			)
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv0", "1-1", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
				insert.TestResults(t, "inv0", "1-2", nil, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "1-1", nil, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "2-1", nil, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "2", nil, pb.TestResult_FAILED),
				insert.TestResults(t, "inv2", "1-2", nil, pb.TestResult_PASSED),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1", "inv2")
			q.Predicate.TestIdRegexp = "1-.+"

			assert.Loosely(t, mustFetchNames(q), should.Match([]string{
				"invocations/inv0/tests/1-1/results/0",
				"invocations/inv0/tests/1-1/results/1",
				"invocations/inv0/tests/1-2/results/0",
				"invocations/inv1/tests/1-1/results/0",
				"invocations/inv2/tests/1-2/results/0",
			}))
		})

		t.Run(`Variant equals`, func(t *ftt.Test) {
			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, t,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
					"TestResultVariantUnion": []string{"k:1", "k:2"},
				}),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, map[string]any{
					"CreateTime": time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				}),
			)
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv0", "1-1", v1, pb.TestResult_PASSED, pb.TestResult_FAILED),
				insert.TestResults(t, "inv0", "1-2", v2, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "1-1", v1, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "2-1", v2, pb.TestResult_PASSED),
				insert.TestResults(t, "inv2", "1-1", v1, pb.TestResult_PASSED),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1", "inv2")
			q.Predicate.Variant = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{Equals: v1},
			}

			assert.Loosely(t, mustFetchNames(q), should.Match([]string{
				"invocations/inv0/tests/1-1/results/0",
				"invocations/inv0/tests/1-1/results/1",
				"invocations/inv1/tests/1-1/results/0",
				"invocations/inv2/tests/1-1/results/0",
			}))
		})

		t.Run(`Variant contains`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
				"TestResultVariantUnion": []string{"k:1", "k:2", "k2:1"},
			}))

			v1 := pbutil.Variant("k", "1")
			v11 := pbutil.Variant("k", "1", "k2", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv0", "1-1", v1, pb.TestResult_PASSED, pb.TestResult_FAILED),
				insert.TestResults(t, "inv0", "1-2", v11, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "1-1", v1, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "2-1", v2, pb.TestResult_PASSED),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")

			t.Run(`Empty`, func(t *ftt.Test) {
				q.Predicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: pbutil.Variant()},
				}

				assert.Loosely(t, mustFetchNames(q), should.Match([]string{
					"invocations/inv0/tests/1-1/results/0",
					"invocations/inv0/tests/1-1/results/1",
					"invocations/inv0/tests/1-2/results/0",
					"invocations/inv1/tests/1-1/results/0",
					"invocations/inv1/tests/2-1/results/0",
				}))
			})

			t.Run(`Non-empty`, func(t *ftt.Test) {
				q.Predicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: v1},
				}

				assert.Loosely(t, mustFetchNames(q), should.Match([]string{
					"invocations/inv0/tests/1-1/results/0",
					"invocations/inv0/tests/1-1/results/1",
					"invocations/inv0/tests/1-2/results/0",
					"invocations/inv1/tests/1-1/results/0",
				}))
			})
		})

		t.Run(`Paging`, func(t *ftt.Test) {
			trs := insert.MakeTestResults("inv1", "DoBaz", &pb.Variant{},
				pb.TestResult_PASSED,
				pb.TestResult_FAILED,
				pb.TestResult_FAILED,
				pb.TestResult_PASSED,
				pb.TestResult_FAILED,
			)
			testutil.MustApply(ctx, t, insert.TestResultMessages(t, trs)...)

			mustReadPage := func(pageToken string, pageSize int, expected []*pb.TestResult) string {
				q2 := q
				q2.PageToken = pageToken
				q2.PageSize = pageSize
				actual, token := mustFetch(q2)
				assert.Loosely(t, actual, should.Match(expected))
				return token
			}

			t.Run(`All results`, func(t *ftt.Test) {
				token := mustReadPage("", 10, trs)
				assert.Loosely(t, token, should.BeEmpty)
			})

			t.Run(`With pagination`, func(t *ftt.Test) {
				token := mustReadPage("", 1, trs[:1])
				assert.Loosely(t, token, should.NotEqual(""))

				token = mustReadPage(token, 4, trs[1:])
				assert.Loosely(t, token, should.NotEqual(""))

				token = mustReadPage(token, 5, nil)
				assert.Loosely(t, token, should.BeEmpty)
			})

			t.Run(`Bad token`, func(t *ftt.Test) {
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()

				t.Run(`From bad position`, func(t *ftt.Test) {
					q.PageToken = "CgVoZWxsbw=="
					_, _, err := q.Fetch(ctx)

					as, ok := appstatus.Get(err)
					assert.That(t, ok, should.BeTrue)
					assert.That(t, as.Code(), should.Equal(codes.InvalidArgument))
					assert.That(t, as.Message(), should.ContainSubstring("invalid page_token"))
				})

				t.Run(`From decoding`, func(t *ftt.Test) {
					q.PageToken = "%%%"
					_, _, err := q.Fetch(ctx)

					as, ok := appstatus.Get(err)
					assert.That(t, ok, should.BeTrue)
					assert.That(t, as.Code(), should.Equal(codes.InvalidArgument))
					assert.That(t, as.Message(), should.ContainSubstring("invalid page_token"))
				})
			})
		})

		t.Run(`Test metadata`, func(t *ftt.Test) {
			expected := insert.MakeTestResults("inv1", "DoBaz", &pb.Variant{}, pb.TestResult_PASSED)
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
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, t, insert.TestResultMessages(t, expected)...)

			actual, _ := mustFetch(q)
			assert.Loosely(t, actual, should.Match(expected))
		})

		t.Run(`Failure reason`, func(t *ftt.Test) {
			expected := insert.MakeTestResults("inv1", "DoFailureReason", &pb.Variant{}, pb.TestResult_FAILED)
			expected[0].FailureReason = &pb.FailureReason{
				PrimaryErrorMessage: "want true, got false",
				Errors: []*pb.FailureReason_Error{
					{Message: "want true, got false"},
					{Message: "want false, got true"},
				},
				TruncatedErrorsCount: 0,
			}
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, t, insert.TestResultMessages(t, expected)...)

			actual, _ := mustFetch(q)
			assert.Loosely(t, actual, should.Match(expected))
		})

		t.Run(`Properties`, func(t *ftt.Test) {
			expected := insert.MakeTestResults("inv1", "WithProperties", &pb.Variant{}, pb.TestResult_PASSED)
			expected[0].Properties = &structpb.Struct{Fields: map[string]*structpb.Value{
				"key": structpb.NewStringValue("value"),
			}}
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, t, insert.TestResultMessages(t, expected)...)

			actual, _ := mustFetch(q)
			assert.Loosely(t, actual, should.Match(expected))
		})

		t.Run(`Skip reason`, func(t *ftt.Test) {
			expected := insert.MakeTestResults("inv1", "WithSkipReason", &pb.Variant{}, pb.TestResult_SKIPPED)
			expected[0].SkipReason = pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, t, insert.TestResultMessages(t, expected)...)

			actual, _ := mustFetch(q)
			assert.Loosely(t, actual, should.Match(expected))
		})

		t.Run(`Variant in the mask`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))

			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv0", "1-1", v1, pb.TestResult_PASSED, pb.TestResult_FAILED),
				insert.TestResults(t, "inv0", "1-2", v2, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "1-1", v1, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "2-1", v2, pb.TestResult_PASSED),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")

			t.Run(`Present`, func(t *ftt.Test) {
				q.Mask = mask.MustFromReadMask(&pb.TestResult{}, "name", "test_id", "variant", "variant_hash")
				results, _ := mustFetch(q)
				for _, r := range results {
					assert.Loosely(t, r.Variant, should.NotBeNil)
					assert.Loosely(t, r.VariantHash, should.NotEqual(""))
				}
			})

			t.Run(`Not present`, func(t *ftt.Test) {
				q.Mask = mask.MustFromReadMask(&pb.TestResult{}, "name", "test_id")
				results, _ := mustFetch(q)
				for _, r := range results {
					assert.Loosely(t, r.Variant, should.BeNil)
					assert.Loosely(t, r.VariantHash, should.BeEmpty)
				}
			})
		})

		t.Run(`TestVariantIdentifier in the mask`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))

			v1 := pbutil.Variant("k", "1")
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv0", "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar", v1, pb.TestResult_PASSED, pb.TestResult_FAILED),
				insert.TestResults(t, "inv1", "://infra/junit_tests!junit:org.chromium.go.luci:ValidationTests#FooBar", v1, pb.TestResult_PASSED),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")

			expectedTVID := &pb.TestIdentifier{
				ModuleName:        "//infra/junit_tests",
				ModuleScheme:      "junit",
				ModuleVariant:     v1,
				ModuleVariantHash: pbutil.VariantHash(v1),
				CoarseName:        "org.chromium.go.luci",
				FineName:          "ValidationTests",
				CaseName:          "FooBar",
			}

			t.Run(`Present`, func(t *ftt.Test) {
				q.Mask = mask.MustFromReadMask(&pb.TestResult{}, "name", "test_id_structured")
				results, _ := mustFetch(q)
				for _, r := range results {
					assert.Loosely(t, r.TestIdStructured, should.Match(expectedTVID))
					assert.Loosely(t, r.TestId, should.BeEmpty)
					assert.Loosely(t, r.Variant, should.BeNil)
					assert.Loosely(t, r.VariantHash, should.BeEmpty)
				}
			})
			t.Run(`Partially Present`, func(t *ftt.Test) {
				q.Mask = mask.MustFromReadMask(&pb.TestResult{}, "name", "test_id_structured.fine_name", "test_id_structured.case_name")
				results, _ := mustFetch(q)
				for _, r := range results {
					assert.Loosely(t, r.TestIdStructured, should.Match(&pb.TestIdentifier{
						FineName: "ValidationTests",
						CaseName: "FooBar",
					}))
					assert.Loosely(t, r.TestId, should.BeEmpty)
					assert.Loosely(t, r.Variant, should.BeNil)
					assert.Loosely(t, r.VariantHash, should.BeEmpty)
				}
			})

			t.Run(`Not present`, func(t *ftt.Test) {
				q.Mask = mask.MustFromReadMask(&pb.TestResult{}, "name")
				results, _ := mustFetch(q)
				for _, r := range results {
					assert.Loosely(t, r.TestIdStructured, should.BeNil)
					assert.Loosely(t, r.TestId, should.BeEmpty)
					assert.Loosely(t, r.Variant, should.BeNil)
					assert.Loosely(t, r.VariantHash, should.BeEmpty)
				}
			})
		})

		t.Run(`Empty list of invocations`, func(t *ftt.Test) {
			q.InvocationIDs = invocations.NewIDSet()
			actual, _ := mustFetch(q)
			assert.Loosely(t, actual, should.HaveLength(0))
		})

		t.Run(`Filter invocations`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, map[string]any{
					"CommonTestIDPrefix": "ninja://browser_tests/",
				}),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, nil),
			)

			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv0", "ninja://browser_tests/1-1", v1, pb.TestResult_PASSED, pb.TestResult_FAILED),
				insert.TestResults(t, "inv0", "1-2", v2, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "ninja://browser_tests/1-1", v1, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "2-1", v2, pb.TestResult_PASSED),
				insert.TestResults(t, "inv2", "ninja://browser_tests/1-1", v1, pb.TestResult_PASSED),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")
			q.Predicate.TestIdRegexp = "ninja://browser_tests/.*"

			results, _ := mustFetch(q)
			for _, r := range results {
				assert.Loosely(t, r.Name, should.NotHavePrefix("invocations/inv2"))
				assert.Loosely(t, r.TestId, should.Equal("ninja://browser_tests/1-1"))
			}
		})

		t.Run(`Query statements`, func(t *ftt.Test) {
			t.Run(`only unexpected exclude exonerated`, func(t *ftt.Test) {
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
				assert.Loosely(t, strings.Join(strings.Fields(st.SQL), " "), should.Equal(strings.Join(strings.Fields(expected), " ")))
			})

			t.Run(`with unexpected filter by testID`, func(t *ftt.Test) {
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
				assert.Loosely(t, strings.Join(strings.Fields(st.SQL), " "), should.Equal(strings.Join(strings.Fields(expected), " ")))
			})
		})
	})
}

func TestToLimitedData(t *testing.T) {
	ctx := context.Background()

	ftt.Run(`ToLimitedData`, t, func(t *ftt.Test) {
		invID := "inv0"
		testID := "FooBar"
		resultID := "123"
		name := pbutil.TestResultName(invID, testID, resultID)
		variant := pbutil.Variant()
		variantHash := pbutil.VariantHash(variant)

		testResult := &pb.TestResult{
			Name:     name,
			TestId:   testID,
			ResultId: resultID,
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "scheme",
				ModuleVariant:     variant,
				ModuleVariantHash: variantHash,
				CoarseName:        "coarse",
				FineName:          "fine",
				CaseName:          "case",
			},
			Variant:     variant,
			Expected:    true,
			Status:      pb.TestStatus_PASS,
			StatusV2:    pb.TestResult_PASSED,
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
				Kind:                pb.FailureReason_CRASH,
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
			SkipReason: pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
			SkippedReason: &pb.SkippedReason{
				Kind:          pb.SkippedReason_DISABLED_AT_DECLARATION,
				ReasonMessage: "skip reason",
			},
			FrameworkExtensions: &pb.FrameworkExtensions{
				WebTest: &pb.WebTest{
					Status:     pb.WebTest_CRASH,
					IsExpected: false,
				},
			},
		}

		expected := &pb.TestResult{
			Name:     name,
			TestId:   testID,
			ResultId: resultID,
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "scheme",
				ModuleVariantHash: variantHash,
				CoarseName:        "coarse",
				FineName:          "fine",
				CaseName:          "case",
			},
			Expected:    true,
			Status:      pb.TestStatus_PASS,
			StatusV2:    pb.TestResult_PASSED,
			StartTime:   &timestamppb.Timestamp{Seconds: 1000, Nanos: 1234},
			Duration:    &durpb.Duration{Seconds: int64(123), Nanos: 234567000},
			VariantHash: variantHash,
			FailureReason: &pb.FailureReason{
				Kind:                pb.FailureReason_CRASH,
				PrimaryErrorMessage: "an error message",
				Errors: []*pb.FailureReason_Error{
					{Message: "an error message"},
					{Message: "an error message2"},
				},
				TruncatedErrorsCount: 0,
			},
			IsMasked:   true,
			SkipReason: pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
			SkippedReason: &pb.SkippedReason{
				Kind:          pb.SkippedReason_DISABLED_AT_DECLARATION,
				ReasonMessage: "skip reason",
			},
			FrameworkExtensions: &pb.FrameworkExtensions{
				WebTest: &pb.WebTest{
					Status:     pb.WebTest_CRASH,
					IsExpected: false,
				},
			},
		}
		t.Run(`masks fields`, func(t *ftt.Test) {
			err := ToLimitedData(ctx, testResult)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, testResult, should.Match(expected))
		})

		t.Run(`truncates primary error message`, func(t *ftt.Test) {
			testResult.FailureReason = &pb.FailureReason{
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
			}

			limitedLongErrMsg := strings.Repeat("a very long error message",
				10)[:limitedReasonLength] + "..."
			limitedLongErrMsg2 := strings.Repeat("a very long error message2",
				10)[:limitedReasonLength] + "..."
			expected.FailureReason = &pb.FailureReason{
				PrimaryErrorMessage: limitedLongErrMsg,
				Errors: []*pb.FailureReason_Error{
					{Message: limitedLongErrMsg},
					{Message: limitedLongErrMsg2},
				},
				TruncatedErrorsCount: 0,
			}

			err := ToLimitedData(ctx, testResult)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, testResult, should.Match(expected))
		})
	})
}
