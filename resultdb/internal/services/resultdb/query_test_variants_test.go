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

package resultdb

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/testvariants"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestQueryTestVariants(t *testing.T) {
	ftt.Run(`QueryTestVariants`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:testlimitedrealm", Permission: rdbperms.PermListLimitedTestResults},
				{Realm: "testproject:testlimitedrealm", Permission: rdbperms.PermListLimitedTestExonerations},
				{Realm: "testproject:testresultrealm", Permission: rdbperms.PermGetTestResult},
				{Realm: "testproject:testexonerationrealm", Permission: rdbperms.PermGetTestExoneration},
			},
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		srv := newTestResultDBService()

		// Set up a single root invocation.
		req := &pb.QueryTestVariantsRequest{
			Parent: "rootInvocations/root-inv1",
		}
		rootInv := rootinvocations.NewBuilder("root-inv1").WithState(pb.RootInvocation_ACTIVE).WithRealm("testproject:testrealm").Build()
		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInv)...)

		t.Run(`request validation`, func(t *ftt.Test) {
			t.Run(`invalid read mask`, func(t *ftt.Test) {
				req.ReadMask = &fieldmaskpb.FieldMask{Paths: []string{"invalid"}}
				_, err := srv.QueryTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`field "invalid" does not exist in message TestVariant`))
			})
			t.Run(`invalid filter`, func(t *ftt.Test) {
				req.Filter = "test OR "

				_, err := srv.QueryTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`filter: expected sequence after AND`))
			})

			// Validation for parent and invocations field are tested in TestDetermineListAccessLevel. Here we just make sure the function is called.
			t.Run(`invalid invocation name`, func(t *ftt.Test) {
				req.Parent = ""
				req.Invocations = []string{"rootInvocations/i0/workunits/wu0"}
				_, err := srv.QueryTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("does not match pattern"))
			})

			// Other request validation is tested in TestValidateQueryTestVariantsRequest. Here we just make sure the function is called.
			t.Run(`negative result_limit`, func(t *ftt.Test) {
				req.ResultLimit = -1
				_, err := srv.QueryTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`result_limit: negative`))
			})
		})

		// Permission is check in TestDetermineListAccessLevel. Here we just make sure the function is called.
		t.Run(`permission denied`, func(t *ftt.Test) {
			rootInv := rootinvocations.NewBuilder("root-invsecret").WithState(pb.RootInvocation_ACTIVE).WithRealm("testproject:secret").Build()
			testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInv)...)
			req.Parent = "rootInvocations/root-invsecret"

			_, err := srv.QueryTestVariants(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.testResults.listLimited in realm of \"rootInvocations/root-invsecret\""))
		})

		t.Run(`e2e`, func(t *ftt.Test) {
			// The Query function has been tested in testvariants.TestQueryTestVariants, here we just need to verify it has been called properly.

			// Set up root invocation, work units and their test results.
			// root-inv1 -> root wu -> wu1
			rootWU := workunits.NewBuilder(rootInv.RootInvocationID, "root").WithState(pb.WorkUnit_ACTIVE).WithRealm("testproject:wurealm").Build()
			wu1 := workunits.
				NewBuilder(rootInv.RootInvocationID, "wu1").
				WithState(pb.WorkUnit_ACTIVE).
				WithRealm("testproject:wurealm").
				WithInstructions(&pb.Instructions{
					Instructions: []*pb.Instruction{
						{
							Id:   "test",
							Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
						},
					},
				}).
				Build()
			tr11 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test1", pbutil.Variant("a", "b"), pb.TestResult_SKIPPED)[0]
			tr12 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test2", pbutil.Variant("c", "d"), pb.TestResult_PASSED)[0]
			tr13 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test3", pbutil.Variant("a", "b"), pb.TestResult_FAILED)[0]
			tr14 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test4", pbutil.Variant("g", "h"), pb.TestResult_EXECUTION_ERRORED)[0]
			tr15 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test5", pbutil.Variant(), pb.TestResult_PRECLUDED)[0]

			testutil.MustApply(ctx, t, testutil.CombineMutations(
				workunits.InsertForTesting(rootWU),
				workunits.InsertForTesting(wu1),
				insert.TestResultMessages(t, []*pb.TestResult{tr11, tr12, tr13, tr14, tr15}),
			)...)

			expectedResultsFirstPage := []*pb.TestResult{tr13, tr14, tr15}
			expectedStatusFirstPage := []struct {
				Status         pb.TestVariantStatus
				StatusOverride pb.TestVerdict_StatusOverride
				StatusV2       pb.TestVerdict_Status
			}{
				{
					Status:         pb.TestVariantStatus_UNEXPECTED,
					StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
					StatusV2:       pb.TestVerdict_FAILED,
				},
				{
					Status:         pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED,
					StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
					StatusV2:       pb.TestVerdict_EXECUTION_ERRORED,
				},
				{
					Status:         pb.TestVariantStatus_UNEXPECTEDLY_SKIPPED,
					StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
					StatusV2:       pb.TestVerdict_PRECLUDED,
				},
			}
			req.Parent = "rootInvocations/root-inv1"
			req.Invocations = nil

			t.Run(`baseline (full access)`, func(t *ftt.Test) {
				res, err := srv.QueryTestVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				for i, tv := range res.TestVariants {
					result := expectedResultsFirstPage[i]

					// Drop fields which are lifted to the test variant level.
					expectedResult := proto.Clone(result).(*pb.TestResult)
					expectedResult.TestId = ""
					expectedResult.TestIdStructured = nil
					expectedResult.TestMetadata = nil
					expectedResult.Variant = nil
					expectedResult.VariantHash = ""

					assert.Loosely(t, tv, should.Match(&pb.TestVariant{
						TestIdStructured: result.TestIdStructured,
						TestId:           result.TestId,
						Variant:          result.Variant,
						VariantHash:      result.VariantHash,
						Status:           expectedStatusFirstPage[i].Status,
						StatusOverride:   expectedStatusFirstPage[i].StatusOverride,
						StatusV2:         expectedStatusFirstPage[i].StatusV2,
						Results:          []*pb.TestResultBundle{{Result: expectedResult}},
						TestMetadata:     result.TestMetadata,
						Instruction:      &pb.VerdictInstruction{Instruction: "rootInvocations/root-inv1/workUnits/wu1/instructions/test"},
						SourcesId:        graph.HashSources(rootInv.Sources).String(),
					}))
				}
				// Check Sources field.
				assert.Loosely(t, res.Sources, should.HaveLength(1))
				assert.Loosely(t, res.Sources[graph.HashSources(rootInv.Sources).String()], should.Match(rootInv.Sources))
				// Check token
				tokenParts, err := pagination.ParseToken(res.NextPageToken)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tokenParts, should.Match([]string{"EXPECTED", "", ""}))
			})

			t.Run(`limited access`, func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestResults},
						{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestExonerations},
					},
				})

				res, err := srv.QueryTestVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				for i, tv := range res.TestVariants {
					result := expectedResultsFirstPage[i]

					// Drop fields which are lifted to the test variant level.
					expectedResult := proto.Clone(result).(*pb.TestResult)
					expectedResult.TestId = ""
					expectedResult.TestIdStructured = nil
					expectedResult.TestMetadata = nil
					expectedResult.Variant = nil
					expectedResult.VariantHash = ""
					err := testresults.ToLimitedData(ctx, expectedResult)
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, tv, should.Match(&pb.TestVariant{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:        result.TestIdStructured.ModuleName,
							ModuleScheme:      result.TestIdStructured.ModuleScheme,
							ModuleVariant:     nil, // Masked
							ModuleVariantHash: result.TestIdStructured.ModuleVariantHash,
							CoarseName:        result.TestIdStructured.CoarseName,
							FineName:          result.TestIdStructured.FineName,
							CaseName:          result.TestIdStructured.CaseName,
						},
						TestId:         result.TestId,
						Variant:        nil, // Masked.
						VariantHash:    result.VariantHash,
						Status:         expectedStatusFirstPage[i].Status,
						StatusOverride: expectedStatusFirstPage[i].StatusOverride,
						StatusV2:       expectedStatusFirstPage[i].StatusV2,
						Results:        []*pb.TestResultBundle{{Result: expectedResult}},
						TestMetadata:   nil, // Masked
						Instruction:    &pb.VerdictInstruction{Instruction: "rootInvocations/root-inv1/workUnits/wu1/instructions/test"},
						SourcesId:      graph.HashSources(rootInv.Sources).String(),
						IsMasked:       true,
					}))
				}
				// Check Sources field.
				assert.Loosely(t, res.Sources, should.HaveLength(1))
				assert.Loosely(t, res.Sources[graph.HashSources(rootInv.Sources).String()], should.Match(rootInv.Sources))
				// verify token
				tokenParts, err := pagination.ParseToken(res.NextPageToken)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tokenParts, should.Match([]string{"EXPECTED", "", ""}))
			})
			t.Run(`valid with status v2 sort order`, func(t *ftt.Test) {
				req.OrderBy = "status_v2_effective"

				res, err := srv.QueryTestVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				tokenParts, err := pagination.ParseToken(res.NextPageToken)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tokenParts, should.Match([]string{"PASSED_OR_SKIPPED", "", ""}))

				assert.Loosely(t, len(res.TestVariants), should.Equal(3))
				assert.Loosely(t, tvStrings(res.TestVariants), should.Match([]string{
					"10/test3/c467ccce5a16dc72/FAILED",
					"20/test4/84cfbe1757a95b22/EXECUTION_ERRORED",
					"20/test5/e3b0c44298fc1c14/PRECLUDED",
				}))

				// Try next page.
				req.PageToken = pagination.Token("PASSED_OR_SKIPPED", "", "")
				res, err = srv.QueryTestVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(res.TestVariants), should.Equal(2))
				assert.Loosely(t, tvStrings(res.TestVariants), should.Match([]string{
					"50/test1/c467ccce5a16dc72/SKIPPED",
					"50/test2/ded11d2f8ab4fbb5/PASSED",
				}))
			})
		})

		t.Run(`e2e legacy`, func(t *ftt.Test) {
			// inv0 -> inv1.
			testutil.MustApply(
				ctx, t,
				insert.InvocationWithInclusions("inv0", pb.Invocation_ACTIVE, map[string]any{
					"Realm":   "testproject:testrealm",
					"Sources": spanutil.Compress(pbutil.MustMarshal(testutil.TestSourcesWithChangelistNumbers(1))),
					"Instructions": spanutil.Compress(pbutil.MustMarshal(&pb.Instructions{
						Instructions: []*pb.Instruction{
							{
								Id:   "test",
								Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
							},
						},
					})),
				}, "inv1", "invmissing")...,
			)
			testutil.MustApply(
				ctx, t,
				insert.Invocation("inv1", pb.Invocation_ACTIVE, map[string]any{
					"Realm":          "testproject:testrealm",
					"InheritSources": true,
				}),
			)
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv0", "T1", nil, pb.TestResult_FAILED),
				insert.TestResultsLegacy(t, "inv0", "T2", nil, pb.TestStatus_FAIL),
				insert.TestResults(t, "inv1", "T3", nil, pb.TestResult_PASSED),
				insert.TestResults(t, "inv1", "T4", nil, pb.TestResult_SKIPPED),
				insert.TestResults(t, "inv1", "T1", pbutil.Variant("a", "b"), pb.TestResult_FAILED, pb.TestResult_PASSED),
				insert.TestExonerations("inv0", "T1", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
			)...)

			// inv2 -> [inv3, inv4, inv5].
			testutil.MustApply(
				ctx, t,
				insert.InvocationWithInclusions("inv2", pb.Invocation_ACTIVE, map[string]any{
					"Realm":   "testproject:testlimitedrealm",
					"Sources": spanutil.Compress(pbutil.MustMarshal(testutil.TestSourcesWithChangelistNumbers(2))),
					"Instructions": spanutil.Compress(pbutil.MustMarshal(&pb.Instructions{
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
					})),
				}, "inv3", "inv4", "inv5")...,
			)
			testutil.MustApply(
				ctx, t,
				insert.Invocation("inv3", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testresultrealm", "InheritSources": true}),
				insert.Invocation("inv4", pb.Invocation_ACTIVE, map[string]any{
					"Realm":   "testproject:testexonerationrealm",
					"Sources": spanutil.Compress(pbutil.MustMarshal(testutil.TestSourcesWithChangelistNumbers(4))),
				}),
				insert.Invocation("inv5", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testlimitedrealm"}),
			)
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.TestResults(t, "inv2", "T1002", pbutil.Variant("k0", "v0"), pb.TestResult_FAILED),
				insert.TestResults(t, "inv3", "T1003", pbutil.Variant("k1", "v1"), pb.TestResult_FAILED),
				insert.TestResultsLegacy(t, "inv4", "T1004", pbutil.Variant("k2", "v2"), pb.TestStatus_FAIL),
				insert.TestResults(t, "inv5", "T1005", pbutil.Variant("k3", "v3"), pb.TestResult_FAILED),
				insert.TestExonerations("inv3", "T1003", pbutil.Variant("k1", "v1"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestExonerations("inv4", "T1004", pbutil.Variant("k2", "v2"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
				insert.TestExonerations("inv5", "T1005", pbutil.Variant("k3", "v3"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
			)...)
			t.Run(`valid with limited list permission`, func(t *ftt.Test) {
				res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
					Invocations: []string{"invocations/inv2"},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(res.TestVariants), should.Equal(4))

				// Check the returned test variants are appropriately masked.
				duration := &durationpb.Duration{Seconds: 0, Nanos: 234567000}
				tags := []*pb.StringPair{
					{Key: "k1", Value: "v1"},
					{Key: "k2", Value: "v2"},
				}
				fx := &pb.FrameworkExtensions{
					WebTest: &pb.WebTest{
						Status:     pb.WebTest_PASS,
						IsExpected: true,
					},
				}
				assert.Loosely(t, res.TestVariants, should.Match([]*pb.TestVariant{
					{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:        "legacy",
							ModuleScheme:      "legacy",
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k0", "v0")),
							CaseName:          "T1002",
						},
						TestId:         "T1002",
						VariantHash:    pbutil.VariantHash(pbutil.Variant("k0", "v0")),
						Status:         pb.TestVariantStatus_UNEXPECTED,
						StatusV2:       pb.TestVerdict_FAILED,
						StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
						Results: []*pb.TestResultBundle{
							{
								Result: &pb.TestResult{
									Name:      "invocations/inv2/tests/T1002/results/0",
									ResultId:  "0",
									Status:    pb.TestStatus_FAIL,
									StatusV2:  pb.TestResult_FAILED,
									StartTime: timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
									Duration:  duration,
									FailureReason: &pb.FailureReason{
										Kind:                pb.FailureReason_ORDINARY,
										PrimaryErrorMessage: "failure reason",
										Errors: []*pb.FailureReason_Error{{
											Message: "failure reason",
										}},
									},
									FrameworkExtensions: fx,
									IsMasked:            true,
								},
							},
						},
						IsMasked:  true,
						SourcesId: graph.HashSources(testutil.TestSourcesWithChangelistNumbers(2)).String(),
					},
					{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:        "legacy",
							ModuleScheme:      "legacy",
							ModuleVariant:     pbutil.Variant("k1", "v1"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k1", "v1")),
							CaseName:          "T1003",
						},
						TestId:         "T1003",
						Variant:        pbutil.Variant("k1", "v1"),
						VariantHash:    pbutil.VariantHash(pbutil.Variant("k1", "v1")),
						Status:         pb.TestVariantStatus_EXONERATED,
						StatusV2:       pb.TestVerdict_FAILED,
						StatusOverride: pb.TestVerdict_EXONERATED,
						Results: []*pb.TestResultBundle{
							{
								Result: &pb.TestResult{
									Name:        "invocations/inv3/tests/T1003/results/0",
									ResultId:    "0",
									Status:      pb.TestStatus_FAIL,
									StatusV2:    pb.TestResult_FAILED,
									StartTime:   timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
									Duration:    duration,
									SummaryHtml: "SummaryHtml",
									FailureReason: &pb.FailureReason{
										Kind:                pb.FailureReason_ORDINARY,
										PrimaryErrorMessage: "failure reason",
										Errors: []*pb.FailureReason_Error{{
											Message: "failure reason",
											Trace:   "trace",
										}},
									},
									Tags: tags,
									Properties: &structpb.Struct{Fields: map[string]*structpb.Value{
										"key": structpb.NewStringValue("value"),
									}},
									FrameworkExtensions: fx,
								},
							},
						},
						Exonerations: []*pb.TestExoneration{
							{
								Name:            "invocations/inv3/tests/T1003/exonerations/0",
								ExplanationHtml: "explanation 0",
								Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
								IsMasked:        true,
							},
						},
						TestMetadata: &pb.TestMetadata{Name: "testname"},
						SourcesId:    graph.HashSources(testutil.TestSourcesWithChangelistNumbers(2)).String(),
						Instruction: &pb.VerdictInstruction{
							Instruction: "invocations/inv2/instructions/test",
						},
					},
					{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:        "legacy",
							ModuleScheme:      "legacy",
							ModuleVariant:     pbutil.Variant("k2", "v2"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k2", "v2")),
							CaseName:          "T1004",
						},
						TestId:         "T1004",
						Variant:        pbutil.Variant("k2", "v2"),
						VariantHash:    pbutil.VariantHash(pbutil.Variant("k2", "v2")),
						Status:         pb.TestVariantStatus_EXONERATED,
						StatusV2:       pb.TestVerdict_FAILED,
						StatusOverride: pb.TestVerdict_EXONERATED,
						Results: []*pb.TestResultBundle{
							{
								Result: &pb.TestResult{
									Name:     "invocations/inv4/tests/T1004/results/0",
									ResultId: "0",
									Status:   pb.TestStatus_FAIL,
									StatusV2: pb.TestResult_FAILED,
									Duration: duration,
									FailureReason: &pb.FailureReason{
										// This is a legacy result, Kind is not populated.
										PrimaryErrorMessage: "failure reason",
										Errors:              []*pb.FailureReason_Error{{Message: "failure reason"}},
									},
									IsMasked: true,
								},
							},
						},
						Exonerations: []*pb.TestExoneration{
							{
								Name:            "invocations/inv4/tests/T1004/exonerations/0",
								ExplanationHtml: "explanation 0",
								Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
							},
						},
						IsMasked:  true,
						SourcesId: graph.HashSources(testutil.TestSourcesWithChangelistNumbers(4)).String(),
					},
					{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName:        "legacy",
							ModuleScheme:      "legacy",
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k3", "v3")),
							CaseName:          "T1005",
						},
						TestId:         "T1005",
						VariantHash:    pbutil.VariantHash(pbutil.Variant("k3", "v3")),
						Status:         pb.TestVariantStatus_EXONERATED,
						StatusV2:       pb.TestVerdict_FAILED,
						StatusOverride: pb.TestVerdict_EXONERATED,
						Results: []*pb.TestResultBundle{
							{
								Result: &pb.TestResult{
									Name:      "invocations/inv5/tests/T1005/results/0",
									ResultId:  "0",
									Status:    pb.TestStatus_FAIL,
									StatusV2:  pb.TestResult_FAILED,
									StartTime: timestamppb.New(time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)),
									Duration:  duration,
									FailureReason: &pb.FailureReason{
										Kind:                pb.FailureReason_ORDINARY,
										PrimaryErrorMessage: "failure reason",
										Errors:              []*pb.FailureReason_Error{{Message: "failure reason"}},
									},
									FrameworkExtensions: fx,
									IsMasked:            true,
								},
							},
						},
						Exonerations: []*pb.TestExoneration{
							{
								Name:            "invocations/inv5/tests/T1005/exonerations/0",
								ExplanationHtml: "explanation 0",
								Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
								IsMasked:        true,
							},
						},
						IsMasked: true,
					},
				}))
				expectedSources := []*pb.Sources{
					testutil.TestSourcesWithChangelistNumbers(2),
					testutil.TestSourcesWithChangelistNumbers(4),
				}
				assert.Loosely(t, res.Sources, should.HaveLength(len(expectedSources)))
				for _, source := range expectedSources {
					assert.Loosely(t, res.Sources[graph.HashSources(source).String()], should.Match(source))
				}
			})

			t.Run(`valid with included invocation`, func(t *ftt.Test) {
				page, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
					Invocations: []string{"invocations/inv0"},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, page.NextPageToken, should.Equal(pagination.Token("EXPECTED", "", "")))

				assert.Loosely(t, len(page.TestVariants), should.Equal(3))
				assert.Loosely(t, tvStrings(page.TestVariants), should.Match([]string{
					"10/T2/e3b0c44298fc1c14/FAILED",
					"30/T1/c467ccce5a16dc72/FLAKY",
					"40/T1/e3b0c44298fc1c14/EXONERATED",
				}))

				expectedSources := testutil.TestSourcesWithChangelistNumbers(1)
				expectedSourceHash := graph.HashSources(expectedSources).String()
				expectedTestResults := [][]*pb.TestResult{
					insert.MakeTestResultsLegacy("inv0", "T2", nil, pb.TestStatus_FAIL),
					insert.MakeTestResults("inv1", "T1", pbutil.Variant("a", "b"), pb.TestResult_FAILED, pb.TestResult_PASSED),
					insert.MakeTestResults("inv0", "T1", &pb.Variant{}, pb.TestResult_FAILED),
				}

				for i, tv := range page.TestVariants {
					assert.Loosely(t, tv.SourcesId, should.Equal(expectedSourceHash))

					expectedResults := expectedTestResults[i]

					assert.Loosely(t, tv.Results, should.HaveLength(len(expectedResults)))
					for j, result := range tv.Results {
						// Drop expectations about fields lifted to the test variant level.
						expectedResults[j].TestId = ""
						expectedResults[j].TestIdStructured = nil
						expectedResults[j].Variant = nil
						expectedResults[j].VariantHash = ""
						expectedResults[j].TestMetadata = nil

						assert.Loosely(t, result.Result, should.Match(expectedResults[j]))
					}
				}

				assert.Loosely(t, page.Sources, should.HaveLength(1))
				assert.Loosely(t, page.Sources[expectedSourceHash], should.Match(expectedSources))
			})

			t.Run(`valid without included invocation`, func(t *ftt.Test) {
				res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
					Invocations: []string{"invocations/inv1"},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.NextPageToken, should.Equal(pagination.Token("EXPECTED", "", "")))

				assert.Loosely(t, len(res.TestVariants), should.Equal(1))
				assert.Loosely(t, tvStrings(res.TestVariants), should.Match([]string{
					"30/T1/c467ccce5a16dc72/FLAKY",
				}))
			})

			t.Run(`valid with status v2 sort order`, func(t *ftt.Test) {
				page, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
					Invocations: []string{"invocations/inv0"},
					OrderBy:     "status_v2_effective",
				})
				assert.Loosely(t, err, should.BeNil)
				tokenParts, err := pagination.ParseToken(page.NextPageToken)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tokenParts, should.Match([]string{"PASSED_OR_SKIPPED", "", ""}))

				assert.Loosely(t, len(page.TestVariants), should.Equal(3))
				assert.Loosely(t, tvStrings(page.TestVariants), should.Match([]string{
					"10/T2/e3b0c44298fc1c14/FAILED",
					"30/T1/c467ccce5a16dc72/FLAKY",
					"40/T1/e3b0c44298fc1c14/EXONERATED",
				}))

				res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
					Invocations: []string{"invocations/inv0"},
					OrderBy:     "status_v2_effective",
					PageToken:   pagination.Token("PASSED_OR_SKIPPED", "", ""),
				})

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(res.TestVariants), should.Equal(2))
				assert.Loosely(t, tvStrings(res.TestVariants), should.Match([]string{
					"50/T3/e3b0c44298fc1c14/PASSED",
					"50/T4/e3b0c44298fc1c14/SKIPPED",
				}))
			})

			t.Run(`too many invocations`, func(t *ftt.Test) {
				_, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
					Invocations: []string{"invocations/inv0", "invocations/inv1"},
					PageSize:    1,
				})

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("invocations: only one invocation is allowed"))
			})

			t.Run(`try next page`, func(t *ftt.Test) {
				res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
					Invocations: []string{"invocations/inv0"},
					PageSize:    3,
					PageToken:   pagination.Token("EXPECTED", "", ""),
				})

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(res.TestVariants), should.Equal(2))
				assert.Loosely(t, tvStrings(res.TestVariants), should.Match([]string{
					"50/T3/e3b0c44298fc1c14/PASSED",
					"50/T4/e3b0c44298fc1c14/SKIPPED",
				}))

				expectedTestResults := [][]*pb.TestResult{
					insert.MakeTestResults("inv1", "T3", &pb.Variant{}, pb.TestResult_PASSED),
					insert.MakeTestResults("inv1", "T4", &pb.Variant{}, pb.TestResult_SKIPPED),
				}

				for i, tv := range res.TestVariants {
					expectedResults := expectedTestResults[i]
					assert.Loosely(t, tv.Results, should.HaveLength(len(expectedResults)))
					for j, result := range tv.Results {
						// Drop expectations about fields lifted to the test variant level.
						expectedResults[j].TestId = ""
						expectedResults[j].TestIdStructured = nil
						expectedResults[j].Variant = nil
						expectedResults[j].VariantHash = ""
						expectedResults[j].TestMetadata = nil

						assert.Loosely(t, result.Result, should.Match(expectedResults[j]))
					}
				}
			})
		})
	})
}

func TestValidateQueryTestVariantsRequest(t *testing.T) {
	ftt.Run(`validateQueryTestVariantsRequest`, t, func(t *ftt.Test) {
		request := &pb.QueryTestVariantsRequest{
			Invocations: []string{"invocations/invx"},
		}
		t.Run(`valid`, func(t *ftt.Test) {
			err := validateQueryTestVariantsRequest(request)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`negative result_limit`, func(t *ftt.Test) {
			request.ResultLimit = -1
			err := validateQueryTestVariantsRequest(request)
			assert.Loosely(t, err, should.ErrLike(`result_limit: negative`))
		})
		t.Run(`invalid page size`, func(t *ftt.Test) {
			request.PageSize = -1
			err := validateQueryTestVariantsRequest(request)
			assert.Loosely(t, err, should.ErrLike(`page_size: negative`))
		})
		t.Run(`order_by`, func(t *ftt.Test) {
			request.OrderBy = "status_v2_effective"

			t.Run(`valid`, func(t *ftt.Test) {
				err := validateQueryTestVariantsRequest(request)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`empty is also valid`, func(t *ftt.Test) {
				request.OrderBy = ""
				err := validateQueryTestVariantsRequest(request)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`invalid syntax`, func(t *ftt.Test) {
				request.OrderBy = "something something"
				err := validateQueryTestVariantsRequest(request)
				assert.Loosely(t, err, should.ErrLike(`order_by: syntax error: 1:11: unexpected token "something"`))
			})
			t.Run(`more than one order by field`, func(t *ftt.Test) {
				request.OrderBy = "test_id, status"
				err := validateQueryTestVariantsRequest(request)
				assert.Loosely(t, err, should.ErrLike(`order_by: more than one order by field is not currently supported`))
			})
			t.Run(`invalid order by field`, func(t *ftt.Test) {
				request.OrderBy = "test_id"
				err := validateQueryTestVariantsRequest(request)
				assert.Loosely(t, err, should.ErrLike(`order_by: order by field must be one of "status" or "status_v2_effective"`))
			})
			t.Run(`descending order`, func(t *ftt.Test) {
				request.OrderBy = "status desc"
				err := validateQueryTestVariantsRequest(request)
				assert.Loosely(t, err, should.ErrLike(`order_by: descending order is not supported`))
			})
		})
	})
}

func TestDetermineListAccessLevel(t *testing.T) {
	ftt.Run("determineListAccessLevel", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		req := &pb.QueryTestVariantsRequest{
			Parent: "rootInvocations/root-inv",
		}
		t.Run(`request validation`, func(t *ftt.Test) {
			t.Run(`invalid invocation name`, func(t *ftt.Test) {
				req.Parent = ""
				req.Invocations = []string{"rootInvocations/i0/workunits/wu0"}
				_, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("does not match pattern"))
			})

			t.Run(`invalid parent name`, func(t *ftt.Test) {
				req.Parent = "invocations/i0"
				_, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("does not match pattern"))
			})

			t.Run(`parent name and invocation name both exist`, func(t *ftt.Test) {
				req.Invocations = []string{"invocations/i0"}
				req.Parent = "rootInvocations/i0"
				_, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`invocations: must not be specified if parent is specified`))
			})

			t.Run(`parent name and invocation name both missing`, func(t *ftt.Test) {
				req.Invocations = []string{}
				req.Parent = ""
				_, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("must specify either parent or invocations"))
			})
		})
		t.Run(`root invocation`, func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			testutil.MustApply(
				ctx, t,
				rootinvocations.InsertForTesting(rootinvocations.NewBuilder("root-inv1").WithRealm("testproject:rootrealm").Build())...,
			)
			req := &pb.QueryTestVariantsRequest{
				Parent: "rootInvocations/root-inv",
			}

			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			t.Run("not found", func(t *ftt.Test) {
				req.Parent = "rootInvocations/root-missing"
				accessLevel, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelInvalid))
			})
			t.Run("access denied", func(t *ftt.Test) {
				t.Run("missing permissionListLimitedTestResults", func(t *ftt.Test) {
					ctx := auth.WithState(ctx, &authtest.FakeState{
						Identity: "user:someone@example.com",
						IdentityPermissions: []authtest.RealmPermission{
							{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedTestExonerations},
						},
					})
					req.Parent = "rootInvocations/root-inv1"
					accessLevel, err := determineListAccessLevel(ctx, req)
					assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
					assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelInvalid))
				})
				t.Run("missing permissionListLimitedTestExonerations", func(t *ftt.Test) {
					ctx := auth.WithState(ctx, &authtest.FakeState{
						Identity: "user:someone@example.com",
						IdentityPermissions: []authtest.RealmPermission{
							{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedTestResults},
						},
					})
					req.Parent = "rootInvocations/root-inv1"
					accessLevel, err := determineListAccessLevel(ctx, req)
					assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
					assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelInvalid))
				})
			})
			t.Run("limited access", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedTestResults},
						{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedTestExonerations},
					},
				})
				req.Parent = "rootInvocations/root-inv1"
				accessLevel, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelLimited))
			})
			t.Run("unrestricted access", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "testproject:rootrealm", Permission: rdbperms.PermListTestResults},
						{Realm: "testproject:rootrealm", Permission: rdbperms.PermListTestExonerations},
					},
				})
				req.Parent = "rootInvocations/root-inv1"
				accessLevel, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelUnrestricted))
			})
		})

		t.Run(`invocations`, func(t *ftt.Test) {
			ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:r1", Permission: rdbperms.PermListArtifacts},
					{Realm: "testproject:r1", Permission: rdbperms.PermListTestExonerations},
					{Realm: "testproject:r1", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:r2", Permission: rdbperms.PermListLimitedTestExonerations},
					{Realm: "testproject:r2", Permission: rdbperms.PermListLimitedTestResults},
					{Realm: "testproject:r2", Permission: rdbperms.PermListTestExonerations},
					{Realm: "testproject:r2", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:r3", Permission: rdbperms.PermListLimitedTestExonerations},
					{Realm: "testproject:r3", Permission: rdbperms.PermListLimitedTestResults},
				},
			})
			testutil.MustApply(
				ctx, t,
				insert.Invocation("i0", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r0"}),
				insert.Invocation("i1", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r1"}),
				insert.Invocation("i2", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r2"}),
				insert.Invocation("i2b", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r2"}),
				insert.Invocation("i3", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r3"}),
				insert.Invocation("i3b", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r3"}),
			)
			req := &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0"},
			}
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			t.Run("not found", func(t *ftt.Test) {
				req.Invocations = []string{"invocations/i2", "invocations/missing"}
				accessLevel, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelInvalid))
			})
			t.Run("access denied", func(t *ftt.Test) {
				req.Invocations = []string{"invocations/i0", "invocations/i2"}

				accessLevel, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelInvalid))
			})
			t.Run("no common access level", func(t *ftt.Test) {
				req.Invocations = []string{"invocations/i1", "invocations/i3"}

				accessLevel, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelInvalid))
			})
			t.Run("limited access", func(t *ftt.Test) {
				req.Invocations = []string{"invocations/i2", "invocations/i2b", "invocations/i3", "invocations/i3b"}

				accessLevel, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelLimited))
			})
			t.Run("full access", func(t *ftt.Test) {
				req.Invocations = []string{"invocations/i1", "invocations/i2", "invocations/i2b"}

				accessLevel, err := determineListAccessLevel(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelUnrestricted))
			})
		})
	})
}
