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
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestQueryTestVariants(t *testing.T) {
	ftt.Run(`QueryTestVariants`, t, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			return page
		}

		fetchAll := func(q *Query, f func(page Page)) {
			for {
				page, err := fetch(q)
				assert.Loosely(t, err, should.BeNil)

				f(page)
				if page.NextPageToken == "" {
					break
				}

				// The page token should always advance, it should
				// never remain the same.
				assert.Loosely(t, page.NextPageToken, should.NotEqual(q.PageToken))
				q.PageToken = page.NextPageToken
			}
		}

		tvStrings := func(tvs []*pb.TestVariant) []string {
			keys := make([]string, len(tvs))
			for i, tv := range tvs {
				instructionName := tv.GetInstruction().GetInstruction()
				statusV2Desc := tv.StatusV2.String()
				if tv.StatusOverride != pb.TestVerdict_NOT_OVERRIDDEN {
					statusV2Desc = tv.StatusOverride.String()
				}
				keys[i] = fmt.Sprintf("%d/%s/%s/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash, statusV2Desc, instructionName)
			}
			return keys
		}

		testutil.MustApply(ctx, t, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, t, insert.Invocation("inv1", pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, t, insert.Invocation("inv2", pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, t, insert.Invocation("inv3", pb.Invocation_ACTIVE, nil))
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.TestResults(t, "inv0", "T1", nil, pb.TestResult_FAILED, pb.TestResult_FAILED),
			insert.TestResults(t, "inv0", "T2", nil, pb.TestResult_PASSED),
			insert.TestResults(t, "inv0", "T5", nil, pb.TestResult_FAILED),
			insert.TestResults(t,
				"inv0", "T6", nil,
				pb.TestResult_PASSED, pb.TestResult_PASSED, pb.TestResult_PASSED,
				pb.TestResult_PASSED, pb.TestResult_PASSED, pb.TestResult_PASSED,
				pb.TestResult_PASSED, pb.TestResult_PASSED, pb.TestResult_PASSED,
				pb.TestResult_PASSED, pb.TestResult_PASSED, pb.TestResult_PASSED,
			),
			insert.TestResults(t, "inv0", "T7", nil, pb.TestResult_SKIPPED),
			insert.TestResults(t, "inv0", "T8", nil, pb.TestResult_PASSED, pb.TestResult_FAILED),
			insert.TestResults(t, "inv0", "T9", nil, pb.TestResult_PASSED),
			insert.TestResults(t, "inv1", "T1", nil, pb.TestResult_FAILED),
			insert.TestResults(t, "inv1", "T2", nil, pb.TestResult_FAILED),
			insert.TestResults(t, "inv1", "T3", nil, pb.TestResult_PASSED, pb.TestResult_PASSED),
			insert.TestResults(t, "inv1", "T4", pbutil.Variant("a", "b"), pb.TestResult_FAILED),
			insert.TestResults(t, "inv1", "T5", pbutil.Variant("a", "b"), pb.TestResult_FAILED, pb.TestResult_PASSED),
			insert.TestResults(t,
				"inv1", "Ty", nil,
				pb.TestResult_FAILED, pb.TestResult_FAILED, pb.TestResult_FAILED,
				pb.TestResult_FAILED, pb.TestResult_FAILED, pb.TestResult_FAILED,
				pb.TestResult_FAILED, pb.TestResult_FAILED, pb.TestResult_FAILED,
				pb.TestResult_FAILED, pb.TestResult_FAILED, pb.TestResult_FAILED,
			),
			// Has both expected and unexpected skips, so should produce a flaky result in verdict status v1 and a skipped result with verdict status v2.
			insert.TestResults(t, "inv1", "Tx", nil, pb.TestResult_EXECUTION_ERRORED, pb.TestResult_SKIPPED),
			insert.TestResults(t, "inv1", "Tz", nil, pb.TestResult_EXECUTION_ERRORED, pb.TestResult_EXECUTION_ERRORED),

			insert.TestExonerations("inv0", "T1", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
				pb.ExonerationReason_NOT_CRITICAL, pb.ExonerationReason_OCCURS_ON_MAINLINE),
			insert.TestExonerations("inv2", "T2", nil, pb.ExonerationReason_UNEXPECTED_PASS),
			insert.TestResults(t, "inv3", "Tw", nil, pb.TestResult_PASSED),
		)...)

		t.Run(`Unexpected works`, func(t *ftt.Test) {
			page := mustFetch(q)
			tvs := page.TestVariants
			tvStrings := tvStrings(tvs)
			assert.Loosely(t, tvStrings, should.Match([]string{
				"10/T4/c467ccce5a16dc72/FAILED/",
				"10/T5/e3b0c44298fc1c14/FAILED/invocations/inv0/instructions/test",
				"10/Ty/e3b0c44298fc1c14/FAILED/",
				"20/Tz/e3b0c44298fc1c14/EXECUTION_ERRORED/",
				"30/T5/c467ccce5a16dc72/FLAKY/",
				"30/T8/e3b0c44298fc1c14/FLAKY/invocations/inv0/instructions/test",
				"30/Tx/e3b0c44298fc1c14/SKIPPED/",
				"40/T1/e3b0c44298fc1c14/EXONERATED/invocations/inv0/instructions/test",
				"40/T2/e3b0c44298fc1c14/FLAKY/",
			}))

			expectedT4Result := insert.MakeTestResults("inv1", "T4", pbutil.Variant("a", "b"), pb.TestResult_FAILED)

			// Test metadata, test ID and variant is reported at the test variant level not the result level.
			tmd := expectedT4Result[0].TestMetadata
			variant := expectedT4Result[0].Variant
			testIDStructured := expectedT4Result[0].TestIdStructured
			expectedT4Result[0].TestMetadata = nil
			expectedT4Result[0].Variant = nil
			expectedT4Result[0].TestId = ""
			expectedT4Result[0].VariantHash = ""
			expectedT4Result[0].TestIdStructured = nil

			assert.Loosely(t, tvs[0].Results, should.Match([]*pb.TestResultBundle{
				{
					Result: expectedT4Result[0],
				},
			}))
			assert.Loosely(t, tvs[0].TestId, should.Equal("T4"))
			assert.Loosely(t, tvs[0].Variant, should.Match(variant))
			assert.Loosely(t, tvs[0].TestIdStructured, should.Match(testIDStructured))
			assert.Loosely(t, tvs[0].TestMetadata, should.Match(tmd))
			assert.Loosely(t, tvs[0].SourcesId, should.Equal(graph.HashSources(sources).String()))

			sort.Slice(tvs[7].Exonerations, func(i, j int) bool {
				return tvs[7].Exonerations[i].ExplanationHtml < tvs[7].Exonerations[j].ExplanationHtml
			})
			assert.Loosely(t, len(tvs[7].Exonerations), should.Equal(3))
			assert.Loosely(t, tvs[7].Exonerations[0], should.Match(&pb.TestExoneration{
				Name:            "invocations/inv0/tests/T1/exonerations/0",
				ExplanationHtml: "explanation 0",
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			}))

			assert.Loosely(t, tvs[7].Exonerations[1], should.Match(&pb.TestExoneration{
				Name:            "invocations/inv0/tests/T1/exonerations/1",
				ExplanationHtml: "explanation 1",
				Reason:          pb.ExonerationReason_NOT_CRITICAL,
			}))
			assert.Loosely(t, tvs[7].Exonerations[2], should.Match(&pb.TestExoneration{
				Name:            "invocations/inv0/tests/T1/exonerations/2",
				ExplanationHtml: "explanation 2",
				Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
			}))
			assert.Loosely(t, len(tvs[8].Exonerations), should.Equal(1))
			assert.Loosely(t, tvs[8].Exonerations[0], should.Match(&pb.TestExoneration{
				Name:            "invocations/inv2/tests/T2/exonerations/0",
				ExplanationHtml: "explanation 0",
				Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
			}))
			assert.Loosely(t, len(tvs[2].Results), should.Equal(10))

			assert.Loosely(t, page.DistinctSources, should.HaveLength(1))
			assert.Loosely(t, page.DistinctSources[graph.HashSources(sources).String()], should.Match(sources))
		})

		t.Run(`Expected works`, func(t *ftt.Test) {
			q.PageToken = pagination.Token("EXPECTED", "", "")
			page := mustFetch(q)
			tvs := page.TestVariants
			assert.Loosely(t, tvStrings(tvs), should.Match([]string{
				"50/T3/e3b0c44298fc1c14/PASSED/",
				"50/T6/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
				"50/T7/e3b0c44298fc1c14/SKIPPED/invocations/inv0/instructions/test",
				"50/T9/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
				"50/Tw/e3b0c44298fc1c14/PASSED/invocations/inv1/instructions/test",
			}))
			assert.Loosely(t, len(tvs[0].Results), should.Equal(2))

			assert.Loosely(t, tvs[0].SourcesId, should.Equal(graph.HashSources(sources).String())) // Sources from inv1.
			assert.Loosely(t, tvs[1].SourcesId, should.BeEmpty)                                    // Sources from inv0.
			assert.Loosely(t, tvs[2].SourcesId, should.BeEmpty)                                    // Sources from inv0.
			assert.Loosely(t, tvs[3].SourcesId, should.BeEmpty)                                    // Sources from inv0.

			assert.Loosely(t, page.DistinctSources, should.HaveLength(1))
			assert.Loosely(t, page.DistinctSources[graph.HashSources(sources).String()], should.Match(sources))
		})

		t.Run(`Field mask works`, func(t *ftt.Test) {
			t.Run(`with minimum field mask`, func(t *ftt.Test) {
				verifyFields := func(tvs []*pb.TestVariant) {
					for _, tv := range tvs {
						// Check all results have and only have .TestId, .VariantHash,
						// .Status populated.
						// Those fields should be included even when not specified.
						assert.Loosely(t, tv.TestId, should.NotBeEmpty)
						assert.Loosely(t, tv.VariantHash, should.NotBeEmpty)
						assert.Loosely(t, tv.Status, should.NotBeZero)
						assert.Loosely(t, tv, should.Match(&pb.TestVariant{
							TestId:         tv.TestId,
							VariantHash:    tv.VariantHash,
							Status:         tv.Status,
							StatusV2:       tv.StatusV2,
							StatusOverride: tv.StatusOverride,
						}))
					}
				}

				t.Run(`with non-expected test variants`, func(t *ftt.Test) {
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

				t.Run(`with expected test variants`, func(t *ftt.Test) {
					// Ensure the last test result (ordered by TestId, then by VariantHash)
					// is expected, so we can verify that the tail is trimmed properly.
					testutil.MustApply(ctx, t,
						insert.TestResults(t, "inv1", "Tz0", nil, pb.TestResult_PASSED)...,
					)
					q.PageToken = pagination.Token("EXPECTED", "", "")

					q.Mask = mask.MustFromReadMask(
						&pb.TestVariant{},
						"test_id",
					)
					page := mustFetch(q)
					tvs := page.TestVariants
					verifyFields(tvs)
					assert.Loosely(t, tvs[len(tvs)-1].TestId, should.Equal("Tz0"))

					// TestId should still be populated even when not specified.
					q.Mask = mask.MustFromReadMask(
						&pb.TestVariant{},
						"status",
					)
					page = mustFetch(q)
					tvs = page.TestVariants
					verifyFields(tvs)
					assert.Loosely(t, tvs[len(tvs)-1].TestId, should.Equal("Tz0"))
				})

			})

			t.Run(`with full field mask`, func(t *ftt.Test) {
				q.Mask = mask.MustFromReadMask(
					&pb.TestVariant{},
					"*",
				)

				verifyFields := func(tvs []*pb.TestVariant) {
					for _, tv := range tvs {
						assert.Loosely(t, tv.TestId, should.NotBeEmpty)
						assert.Loosely(t, tv.VariantHash, should.NotBeEmpty)
						assert.Loosely(t, tv.TestIdStructured, should.NotBeNil)
						assert.Loosely(t, tv.TestIdStructured.ModuleName, should.NotBeEmpty)
						assert.Loosely(t, tv.TestIdStructured.ModuleScheme, should.NotBeEmpty)
						assert.Loosely(t, tv.TestIdStructured.ModuleVariantHash, should.NotBeEmpty)
						assert.Loosely(t, tv.TestIdStructured.CaseName, should.NotBeEmpty)

						assert.That(t, tv.Status, should.NotEqual(pb.TestVariantStatus_TEST_VARIANT_STATUS_UNSPECIFIED))
						assert.That(t, tv.StatusV2, should.NotEqual(pb.TestVerdict_STATUS_UNSPECIFIED))
						assert.That(t, tv.StatusOverride, should.NotEqual(pb.TestVerdict_STATUS_OVERRIDE_UNSPECIFIED))
						assert.Loosely(t, tv.Results, should.NotBeEmpty)
						assert.Loosely(t, tv.TestMetadata, should.NotBeNil)
						if tv.TestId == "T3" {
							assert.Loosely(t, tv.SourcesId, should.NotBeEmpty)
						}

						if tv.Status == pb.TestVariantStatus_EXONERATED || tv.StatusOverride == pb.TestVerdict_EXONERATED {
							assert.Loosely(t, tv.Exonerations, should.NotBeEmpty)
						}

						for _, result := range tv.Results {
							assert.Loosely(t, result.Result.Name, should.NotBeEmpty)
							assert.Loosely(t, result.Result.ResultId, should.NotBeEmpty)
							assert.Loosely(t, result.Result.Status, should.NotBeZero)
							assert.Loosely(t, result.Result.StatusV2, should.NotBeZero)
							assert.Loosely(t, result.Result.SummaryHtml, should.NotBeBlank)
							assert.Loosely(t, result.Result.Duration, should.NotBeNil)
							assert.Loosely(t, result.Result.Tags, should.NotBeNil)
							if tv.TestId == "T4" {
								assert.Loosely(t, result.Result.FailureReason, should.NotBeNil)
								assert.Loosely(t, result.Result.Properties, should.NotBeNil)
							}
							if tv.TestId == "T7" {
								assert.Loosely(t, result.Result.SkippedReason, should.NotBeNil)
							}
							assert.Loosely(t, result.Result.FrameworkExtensions, should.NotBeNil)
							assert.Loosely(t, result, should.Match(&pb.TestResultBundle{
								Result: &pb.TestResult{
									Name:                result.Result.Name,
									ResultId:            result.Result.ResultId,
									Expected:            result.Result.Expected,
									Status:              result.Result.Status,
									StatusV2:            result.Result.StatusV2,
									SummaryHtml:         result.Result.SummaryHtml,
									StartTime:           result.Result.StartTime,
									Duration:            result.Result.Duration,
									Tags:                result.Result.Tags,
									FailureReason:       result.Result.FailureReason,
									Properties:          result.Result.Properties,
									SkipReason:          result.Result.SkipReason,
									SkippedReason:       result.Result.SkippedReason,
									FrameworkExtensions: result.Result.FrameworkExtensions,
								},
							}))
						}

						for _, exoneration := range tv.Exonerations {
							assert.Loosely(t, exoneration.ExplanationHtml, should.NotBeEmpty)
							if tv.TestId != "T2" {
								assert.Loosely(t, exoneration.Reason, should.NotBeZero)
							}
							assert.Loosely(t, exoneration, should.Match(&pb.TestExoneration{
								Name:            exoneration.Name,
								ExplanationHtml: exoneration.ExplanationHtml,
								Reason:          exoneration.Reason,
							}))
						}

						assert.Loosely(t, tv, should.Match(&pb.TestVariant{
							TestId:           tv.TestId,
							VariantHash:      tv.VariantHash,
							TestIdStructured: tv.TestIdStructured,
							Status:           tv.Status,
							StatusV2:         tv.StatusV2,
							StatusOverride:   tv.StatusOverride,
							Results:          tv.Results,
							Exonerations:     tv.Exonerations,
							Variant:          tv.Variant,
							TestMetadata:     tv.TestMetadata,
							SourcesId:        tv.SourcesId,
							Instruction:      tv.Instruction,
						}))
					}
				}

				t.Run(`with non-expected test variants`, func(t *ftt.Test) {
					page := mustFetch(q)
					tvs := page.TestVariants
					verifyFields(tvs)
				})

				t.Run(`with expected test variants`, func(t *ftt.Test) {
					// Ensure the last test result (ordered by TestId, then by VariantHash)
					// is expected, so we can verify that the tail is trimmed properly.
					testutil.MustApply(ctx, t,
						insert.TestResults(t, "inv1", "Tz0", nil, pb.TestResult_PASSED)...,
					)
					q.PageToken = pagination.Token("EXPECTED", "", "")
					page := mustFetch(q)
					tvs := page.TestVariants
					verifyFields(tvs)
					assert.Loosely(t, tvs[len(tvs)-1].TestId, should.Equal("Tz0"))
				})
			})
		})

		t.Run(`paging works`, func(t *ftt.Test) {
			page := func(token string, expectedTVLen int32, expectedTVStrings []string) string {
				q.PageToken = token
				page := mustFetch(q)
				assert.Loosely(t, tvStrings(page.TestVariants), should.Match(expectedTVStrings))
				return page.NextPageToken
			}

			q.PageSize = 15
			nextToken := page("", 8, []string{
				"10/T4/c467ccce5a16dc72/FAILED/",
				"10/T5/e3b0c44298fc1c14/FAILED/invocations/inv0/instructions/test",
				"10/Ty/e3b0c44298fc1c14/FAILED/",
				"20/Tz/e3b0c44298fc1c14/EXECUTION_ERRORED/",
				"30/T5/c467ccce5a16dc72/FLAKY/",
				"30/T8/e3b0c44298fc1c14/FLAKY/invocations/inv0/instructions/test",
				"30/Tx/e3b0c44298fc1c14/SKIPPED/",
				"40/T1/e3b0c44298fc1c14/EXONERATED/invocations/inv0/instructions/test",
				"40/T2/e3b0c44298fc1c14/FLAKY/",
			})
			assert.Loosely(t, nextToken, should.Equal(pagination.Token("EXPECTED", "", "")))

			nextToken = page(nextToken, 1, []string{
				"50/T3/e3b0c44298fc1c14/PASSED/",
			})
			assert.Loosely(t, nextToken, should.Equal(pagination.Token("EXPECTED", "T5", "e3b0c44298fc1c14")))

			nextToken = page(nextToken, 2, []string{
				"50/T6/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
				"50/T7/e3b0c44298fc1c14/SKIPPED/invocations/inv0/instructions/test",
			})
			assert.Loosely(t, nextToken, should.Equal(pagination.Token("EXPECTED", "T8", "e3b0c44298fc1c14")))

			nextToken = page(nextToken, 2, []string{"50/T9/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test", "50/Tw/e3b0c44298fc1c14/PASSED/invocations/inv1/instructions/test"})
			assert.Loosely(t, nextToken, should.Equal("CghFWFBFQ1RFRAoCVHkKEGUzYjBjNDQyOThmYzFjMTQ="))

			nextToken = page(nextToken, 0, []string{})
			assert.Loosely(t, nextToken, should.BeEmpty)
		})

		t.Run(`Page Token`, func(t *ftt.Test) {
			t.Run(`wrong number of parts`, func(t *ftt.Test) {
				q.PageToken = pagination.Token("testId", "variantHash")
				_, err := q.Fetch(ctx)
				as, ok := appstatus.Get(err)
				assert.That(t, ok, should.BeTrue)
				assert.That(t, as.Code(), should.Equal(codes.InvalidArgument))
				assert.That(t, as.Message(), should.ContainSubstring("invalid page_token"))
			})

			t.Run(`first part not tvStatus`, func(t *ftt.Test) {
				q.PageToken = pagination.Token("50", "testId", "variantHash")
				_, err := q.Fetch(ctx)
				as, ok := appstatus.Get(err)
				assert.That(t, ok, should.BeTrue)
				assert.That(t, as.Code(), should.Equal(codes.InvalidArgument))
				assert.That(t, as.Message(), should.ContainSubstring("invalid page_token"))
			})
		})

		t.Run(`status filter works`, func(t *ftt.Test) {
			t.Run(`only unexpected`, func(t *ftt.Test) {
				q.Predicate = &pb.TestVariantPredicate{Status: pb.TestVariantStatus_UNEXPECTED}
				page := mustFetch(q)
				tvStrings := tvStrings(page.TestVariants)
				assert.Loosely(t, tvStrings, should.Match([]string{
					"10/T4/c467ccce5a16dc72/FAILED/",
					"10/T5/e3b0c44298fc1c14/FAILED/invocations/inv0/instructions/test",
					"10/Ty/e3b0c44298fc1c14/FAILED/",
				}))
				assert.Loosely(t, page.NextPageToken, should.BeEmpty)
			})

			t.Run(`only expected`, func(t *ftt.Test) {
				q.Predicate = &pb.TestVariantPredicate{Status: pb.TestVariantStatus_EXPECTED}
				page := mustFetch(q)
				assert.Loosely(t, tvStrings(page.TestVariants), should.Match([]string{
					"50/T3/e3b0c44298fc1c14/PASSED/",
					"50/T6/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
					"50/T7/e3b0c44298fc1c14/SKIPPED/invocations/inv0/instructions/test",
					"50/T9/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
					"50/Tw/e3b0c44298fc1c14/PASSED/invocations/inv1/instructions/test",
				}))
				assert.Loosely(t, len(page.TestVariants[0].Results), should.Equal(2))
			})

			t.Run(`any unexpected or exonerated`, func(t *ftt.Test) {
				q.Predicate = &pb.TestVariantPredicate{Status: pb.TestVariantStatus_UNEXPECTED_MASK}
				page := mustFetch(q)
				assert.Loosely(t, tvStrings(page.TestVariants), should.Match([]string{
					"10/T4/c467ccce5a16dc72/FAILED/",
					"10/T5/e3b0c44298fc1c14/FAILED/invocations/inv0/instructions/test",
					"10/Ty/e3b0c44298fc1c14/FAILED/",
					"20/Tz/e3b0c44298fc1c14/EXECUTION_ERRORED/",
					"30/T5/c467ccce5a16dc72/FLAKY/",
					"30/T8/e3b0c44298fc1c14/FLAKY/invocations/inv0/instructions/test",
					"30/Tx/e3b0c44298fc1c14/SKIPPED/",
					"40/T1/e3b0c44298fc1c14/EXONERATED/invocations/inv0/instructions/test",
					"40/T2/e3b0c44298fc1c14/FLAKY/",
				}))
			})
		})

		t.Run(`ResultLimit works`, func(t *ftt.Test) {
			q.ResultLimit = 2
			page := mustFetch(q)

			for _, tv := range page.TestVariants {
				assert.Loosely(t, len(tv.Results), should.BeLessThanOrEqual(q.ResultLimit))
			}
		})

		t.Run(`ResponseLimitBytes works`, func(t *ftt.Test) {
			q.ResponseLimitBytes = 1

			var allTVs []*pb.TestVariant
			fetchAll(q, func(page Page) {
				// Expect at most one test variant per page.
				assert.Loosely(t, len(page.TestVariants), should.BeLessThanOrEqual(1))
				allTVs = append(allTVs, page.TestVariants...)
			})

			// All test variants should be returned.
			assert.Loosely(t, tvStrings(allTVs), should.Match([]string{
				"10/T4/c467ccce5a16dc72/FAILED/",
				"10/T5/e3b0c44298fc1c14/FAILED/invocations/inv0/instructions/test",
				"10/Ty/e3b0c44298fc1c14/FAILED/",
				"20/Tz/e3b0c44298fc1c14/EXECUTION_ERRORED/",
				"30/T5/c467ccce5a16dc72/FLAKY/",
				"30/T8/e3b0c44298fc1c14/FLAKY/invocations/inv0/instructions/test",
				"30/Tx/e3b0c44298fc1c14/SKIPPED/",
				"40/T1/e3b0c44298fc1c14/EXONERATED/invocations/inv0/instructions/test",
				"40/T2/e3b0c44298fc1c14/FLAKY/",
				"50/T3/e3b0c44298fc1c14/PASSED/",
				"50/T6/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
				"50/T7/e3b0c44298fc1c14/SKIPPED/invocations/inv0/instructions/test",
				"50/T9/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
				"50/Tw/e3b0c44298fc1c14/PASSED/invocations/inv1/instructions/test",
			}))
		})

		t.Run(`PageSize works`, func(t *ftt.Test) {
			q.PageSize = 1

			var allTVs []*pb.TestVariant
			fetchAll(q, func(page Page) {
				// Expect at most one test variant per page.
				assert.Loosely(t, len(page.TestVariants), should.BeLessThanOrEqual(1))
				allTVs = append(allTVs, page.TestVariants...)
			})

			// All test variants should be returned.
			assert.Loosely(t, tvStrings(allTVs), should.Match([]string{
				"10/T4/c467ccce5a16dc72/FAILED/",
				"10/T5/e3b0c44298fc1c14/FAILED/invocations/inv0/instructions/test",
				"10/Ty/e3b0c44298fc1c14/FAILED/",
				"20/Tz/e3b0c44298fc1c14/EXECUTION_ERRORED/",
				"30/T5/c467ccce5a16dc72/FLAKY/",
				"30/T8/e3b0c44298fc1c14/FLAKY/invocations/inv0/instructions/test",
				"30/Tx/e3b0c44298fc1c14/SKIPPED/",
				"40/T1/e3b0c44298fc1c14/EXONERATED/invocations/inv0/instructions/test",
				"40/T2/e3b0c44298fc1c14/FLAKY/",
				"50/T3/e3b0c44298fc1c14/PASSED/",
				"50/T6/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
				"50/T7/e3b0c44298fc1c14/SKIPPED/invocations/inv0/instructions/test",
				"50/T9/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
				"50/Tw/e3b0c44298fc1c14/PASSED/invocations/inv1/instructions/test",
			}))
		})

		t.Run(`Querying with limited access works`, func(t *ftt.Test) {
			q.AccessLevel = AccessLevelLimited

			t.Run(`with only limited access`, func(t *ftt.Test) {
				page := mustFetch(q)
				assert.Loosely(t, tvStrings(page.TestVariants), should.Match([]string{
					"10/T4/c467ccce5a16dc72/FAILED/",
					"10/T5/e3b0c44298fc1c14/FAILED/invocations/inv0/instructions/test",
					"10/Ty/e3b0c44298fc1c14/FAILED/",
					"20/Tz/e3b0c44298fc1c14/EXECUTION_ERRORED/",
					"30/T5/c467ccce5a16dc72/FLAKY/",
					"30/T8/e3b0c44298fc1c14/FLAKY/invocations/inv0/instructions/test",
					"30/Tx/e3b0c44298fc1c14/SKIPPED/",
					"40/T1/e3b0c44298fc1c14/EXONERATED/invocations/inv0/instructions/test",
					"40/T2/e3b0c44298fc1c14/FLAKY/",
				}))

				// Check the test variant and its test results and test exonerations
				// have all been masked.
				for _, tv := range page.TestVariants {
					assert.Loosely(t, tv.TestMetadata, should.BeNil)
					assert.Loosely(t, tv.Variant, should.BeNil)
					assert.Loosely(t, tv.TestIdStructured.ModuleVariant, should.BeNil)
					assert.Loosely(t, tv.IsMasked, should.BeTrue)

					for _, tr := range tv.Results {
						assert.Loosely(t, tr.Result.IsMasked, should.BeTrue)
					}

					for _, te := range tv.Exonerations {
						assert.Loosely(t, te.IsMasked, should.BeTrue)
					}
				}

				// Check test results and exonerations for data that should
				// still be available after masking to limited data.
				assert.Loosely(t, page.TestVariants[0].Results, should.Match([]*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:      "invocations/inv1/tests/T4/results/0",
							ResultId:  "0",
							Expected:  false,
							Status:    pb.TestStatus_FAIL,
							StatusV2:  pb.TestResult_FAILED,
							StartTime: timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
							Duration:  &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							FailureReason: &pb.FailureReason{
								Kind:                pb.FailureReason_ORDINARY,
								PrimaryErrorMessage: "failure reason",
								Errors: []*pb.FailureReason_Error{
									{Message: "failure reason"},
								},
								TruncatedErrorsCount: 0,
							},
							FrameworkExtensions: &pb.FrameworkExtensions{
								WebTest: &pb.WebTest{
									Status:     pb.WebTest_PASS,
									IsExpected: true,
								},
							},
							IsMasked: true,
						},
					},
				}))
				assert.Loosely(t, len(page.TestVariants[8].Exonerations), should.Equal(1))
				assert.Loosely(t, page.TestVariants[8].Exonerations[0], should.Match(&pb.TestExoneration{
					Name:            "invocations/inv2/tests/T2/exonerations/0",
					ExplanationHtml: "explanation 0",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
					IsMasked:        true,
				}))
				assert.Loosely(t, len(page.TestVariants[2].Results), should.Equal(10))
			})

			t.Run(`with unrestricted test result access`, func(t *ftt.Test) {
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
				assert.Loosely(t, tvStrings(tvs), should.Match([]string{
					"10/T4/c467ccce5a16dc72/FAILED/",
					"10/T5/e3b0c44298fc1c14/FAILED/",
					"10/Ty/e3b0c44298fc1c14/FAILED/",
					"20/Tz/e3b0c44298fc1c14/EXECUTION_ERRORED/",
					"30/T5/c467ccce5a16dc72/FLAKY/",
					"30/T8/e3b0c44298fc1c14/FLAKY/",
					"30/Tx/e3b0c44298fc1c14/SKIPPED/",
					"40/T1/e3b0c44298fc1c14/EXONERATED/",
					"40/T2/e3b0c44298fc1c14/FLAKY/",
				}))

				// Check all the test exonerations have been masked.
				for _, tv := range tvs {
					for _, te := range tv.Exonerations {
						assert.Loosely(t, te.IsMasked, should.BeTrue)
					}
				}

				// The only test result for the below test variant should be unmasked
				// as the user has access to unrestricted test results in inv1's realm.
				// Thus, the test variant should also be unmasked.
				assert.Loosely(t, tvs[0].IsMasked, should.BeFalse)
				assert.Loosely(t, tvs[0].TestMetadata, should.Match(&pb.TestMetadata{Name: "testname"}))
				assert.Loosely(t, tvs[0].Variant, should.NotBeNil)
				assert.Loosely(t, tvs[0].TestIdStructured.ModuleVariant, should.NotBeNil)
				assert.Loosely(t, tvs[0].Results, should.Match([]*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:        "invocations/inv1/tests/T4/results/0",
							ResultId:    "0",
							Expected:    false,
							Status:      pb.TestStatus_FAIL,
							StatusV2:    pb.TestResult_FAILED,
							StartTime:   timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
							Duration:    &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							SummaryHtml: "SummaryHtml",
							FailureReason: &pb.FailureReason{
								Kind:                pb.FailureReason_ORDINARY,
								PrimaryErrorMessage: "failure reason",
								Errors: []*pb.FailureReason_Error{
									{Message: "failure reason"},
								},
								TruncatedErrorsCount: 0,
							},
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"key": structpb.NewStringValue("value"),
								},
							},
							Tags: []*pb.StringPair{
								{Key: "k1", Value: "v1"},
								{Key: "k2", Value: "v2"},
							},
							FrameworkExtensions: &pb.FrameworkExtensions{
								WebTest: &pb.WebTest{
									Status:     pb.WebTest_PASS,
									IsExpected: true,
								},
							},
						},
					},
				}))

				// Both test results for the below test variant should have been masked.
				// Thus, the test variant should be masked.
				assert.Loosely(t, tvs[5].IsMasked, should.BeTrue)
				assert.Loosely(t, tvs[5].TestMetadata, should.BeNil)
				assert.Loosely(t, tvs[5].Variant, should.BeNil)
				assert.Loosely(t, tvs[5].TestIdStructured.ModuleVariant, should.BeNil)
				assert.Loosely(t, len(tvs[5].Results), should.Equal(2))
				for _, tr := range tvs[5].Results {
					assert.Loosely(t, tr.Result.IsMasked, should.BeTrue)
				}

				// The below test variant should have both a masked and
				// unmasked test result due to the different invocations' realms.
				// Thus, the test variant should be unmasked.
				assert.Loosely(t, tvs[8].IsMasked, should.BeFalse)
				assert.Loosely(t, tvs[8].Results, should.Match([]*pb.TestResultBundle{
					{
						Result: &pb.TestResult{
							Name:      "invocations/inv1/tests/T2/results/0",
							ResultId:  "0",
							Expected:  false,
							Status:    pb.TestStatus_FAIL,
							StatusV2:  pb.TestResult_FAILED,
							StartTime: timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
							Duration:  &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							FailureReason: &pb.FailureReason{
								Kind: pb.FailureReason_ORDINARY,
								Errors: []*pb.FailureReason_Error{
									{Message: "failure reason"},
								},
								PrimaryErrorMessage: "failure reason",
							},
							SummaryHtml: "SummaryHtml",
							Tags: []*pb.StringPair{
								{Key: "k1", Value: "v1"},
								{Key: "k2", Value: "v2"},
							},
							Properties: &structpb.Struct{Fields: map[string]*structpb.Value{
								"key": structpb.NewStringValue("value"),
							}},
							FrameworkExtensions: &pb.FrameworkExtensions{
								WebTest: &pb.WebTest{
									Status:     pb.WebTest_PASS,
									IsExpected: true,
								},
							},
						},
					},
					{
						Result: &pb.TestResult{
							Name:      "invocations/inv0/tests/T2/results/0",
							ResultId:  "0",
							Expected:  true,
							Status:    pb.TestStatus_PASS,
							StatusV2:  pb.TestResult_PASSED,
							StartTime: timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
							Duration:  &durationpb.Duration{Seconds: 0, Nanos: 234567000},
							FrameworkExtensions: &pb.FrameworkExtensions{
								WebTest: &pb.WebTest{
									Status:     pb.WebTest_PASS,
									IsExpected: true,
								},
							},
							IsMasked: true,
						},
					},
				}))
				assert.Loosely(t, len(tvs[8].Exonerations), should.Equal(1))
				assert.Loosely(t, tvs[8].Exonerations[0], should.Match(&pb.TestExoneration{
					Name:            "invocations/inv2/tests/T2/exonerations/0",
					ExplanationHtml: "explanation 0",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
					IsMasked:        true,
				}))
			})
		})

		t.Run(`Empty Invocation works`, func(t *ftt.Test) {
			reachableInvs := graph.NewReachableInvocations()
			reachableInvs.Invocations["invnotexists"] = graph.ReachableInvocation{}
			q.ReachableInvocations = reachableInvs

			var allTVs []*pb.TestVariant
			fetchAll(q, func(page Page) {
				allTVs = append(allTVs, page.TestVariants...)
			})
			assert.Loosely(t, allTVs, should.HaveLength(0))
		})
		t.Run(`Order by works`, func(t *ftt.Test) {
			q.OrderBy = SortOrderStatusV2Effective

			expectedTVs := []string{
				"10/T4/c467ccce5a16dc72/FAILED/",
				"10/T5/e3b0c44298fc1c14/FAILED/invocations/inv0/instructions/test",
				"10/Ty/e3b0c44298fc1c14/FAILED/",
				"20/Tz/e3b0c44298fc1c14/EXECUTION_ERRORED/",
				"40/T2/e3b0c44298fc1c14/FLAKY/",
				"30/T5/c467ccce5a16dc72/FLAKY/",
				"30/T8/e3b0c44298fc1c14/FLAKY/invocations/inv0/instructions/test",
				"40/T1/e3b0c44298fc1c14/EXONERATED/invocations/inv0/instructions/test",
				"50/T3/e3b0c44298fc1c14/PASSED/",
				"50/T6/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
				"50/T7/e3b0c44298fc1c14/SKIPPED/invocations/inv0/instructions/test",
				"50/T9/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
				"50/Tw/e3b0c44298fc1c14/PASSED/invocations/inv1/instructions/test",
				"30/Tx/e3b0c44298fc1c14/SKIPPED/",
			}
			t.Run(`Fetch all`, func(t *ftt.Test) {
				var allTVs []*pb.TestVariant
				fetchAll(q, func(page Page) {
					allTVs = append(allTVs, page.TestVariants...)
				})
				// All test variants should be returned.
				assert.Loosely(t, tvStrings(allTVs), should.Match(expectedTVs))
			})
			t.Run(`With minimum page size`, func(t *ftt.Test) {
				// Stress test the pagination.
				q.PageSize = 1

				var allTVs []*pb.TestVariant
				fetchAll(q, func(page Page) {
					allTVs = append(allTVs, page.TestVariants...)
				})
				// All test variants should be returned.
				assert.Loosely(t, tvStrings(allTVs), should.Match(expectedTVs))
			})
			t.Run(`With minimum page and result size`, func(t *ftt.Test) {
				// Stress test all aspects. Cutting down the result limit
				// may slightly harm reproduction instructions and the
				// v1 verdicts.
				q.ResultLimit = 1
				q.PageSize = 1

				var allTVs []*pb.TestVariant
				fetchAll(q, func(page Page) {
					allTVs = append(allTVs, page.TestVariants...)
				})
				// All test variants should be returned.
				assert.Loosely(t, tvStrings(allTVs), should.Match([]string{
					"10/T4/c467ccce5a16dc72/FAILED/",
					"10/T5/e3b0c44298fc1c14/FAILED/invocations/inv0/instructions/test",
					"10/Ty/e3b0c44298fc1c14/FAILED/",
					"20/Tz/e3b0c44298fc1c14/EXECUTION_ERRORED/",
					"40/T2/e3b0c44298fc1c14/FLAKY/",
					"30/T5/c467ccce5a16dc72/FLAKY/",
					"30/T8/e3b0c44298fc1c14/FLAKY/invocations/inv0/instructions/test",
					"40/T1/e3b0c44298fc1c14/EXONERATED/invocations/inv0/instructions/test",
					"50/T3/e3b0c44298fc1c14/PASSED/",
					"50/T6/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
					"50/T7/e3b0c44298fc1c14/SKIPPED/invocations/inv0/instructions/test",
					"50/T9/e3b0c44298fc1c14/PASSED/invocations/inv0/instructions/test",
					"50/Tw/e3b0c44298fc1c14/PASSED/invocations/inv1/instructions/test",
					"50/Tx/e3b0c44298fc1c14/SKIPPED/", // V1 status should be flaky, now reported as expected.
				}))
			})
			t.Run(`With limited access`, func(t *ftt.Test) {
				q.AccessLevel = AccessLevelLimited
				q.PageSize = 1 // To stress test pagination in combination with masking.

				var allTVs []*pb.TestVariant
				fetchAll(q, func(page Page) {
					allTVs = append(allTVs, page.TestVariants...)
				})
				// All test variants should be returned.
				assert.Loosely(t, tvStrings(allTVs), should.Match(expectedTVs))
			})
			t.Run(`With minimum field mask`, func(t *ftt.Test) {
				q.Mask = mask.MustFromReadMask(
					&pb.TestVariant{},
					"test_id",
				)
				q.PageSize = 1 // To stress test pagination in combination with masking.

				for i := range expectedTVs {
					// Remove expectations for reproduction instructions.
					expectedTVs[i] = strings.Replace(expectedTVs[i], "/invocations/inv0/instructions/test", "/", 1)
					expectedTVs[i] = strings.Replace(expectedTVs[i], "/invocations/inv1/instructions/test", "/", 1)
				}

				var allTVs []*pb.TestVariant
				fetchAll(q, func(page Page) {
					allTVs = append(allTVs, page.TestVariants...)
				})
				// All test variants should be returned.
				assert.Loosely(t, tvStrings(allTVs), should.Match(expectedTVs))
			})
		})
	})
}

func TestAdjustResultLimit(t *testing.T) {
	t.Parallel()
	ftt.Run(`AdjustResultLimit`, t, func(t *ftt.Test) {
		t.Run(`OK`, func(t *ftt.Test) {
			assert.Loosely(t, AdjustResultLimit(50), should.Equal(50))
		})
		t.Run(`Too big`, func(t *ftt.Test) {
			assert.Loosely(t, AdjustResultLimit(1e6), should.Equal(resultLimitMax))
		})
		t.Run(`Missing or 0`, func(t *ftt.Test) {
			assert.Loosely(t, AdjustResultLimit(0), should.Equal(resultLimitDefault))
		})
	})
}
