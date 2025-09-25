// Copyright 2021 The LUCI Authors.
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
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func variantHash(pairs ...string) string {
	return pbutil.VariantHash(pbutil.Variant(pairs...))
}

func tvStrings(tvs []*pb.TestVariant) []string {
	tvStrings := make([]string, len(tvs))
	for i, tv := range tvs {
		statusV2Desc := tv.StatusV2.String()
		if tv.StatusOverride != pb.TestVerdict_NOT_OVERRIDDEN {
			statusV2Desc = tv.StatusOverride.String()
		}
		tvStrings[i] = fmt.Sprintf("%d/%s/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash, statusV2Desc)
	}
	return tvStrings
}

func TestBatchGetTestVariants(t *testing.T) {
	ftt.Run(`BatchGetTestVariants`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},      // For legacy invocation.
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations}, // For legacy invocation.
				{Realm: "testproject:rootrealm", Permission: rdbperms.PermGetTestResult},
				{Realm: "testproject:rootrealm", Permission: rdbperms.PermGetTestExoneration},
			},
		})

		testInstructions := &pb.Instructions{
			Instructions: []*pb.Instruction{
				{
					Id:   "test",
					Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
				},
			},
		}
		// Set up invocations and their test results (legacy).
		// i0 -> i1
		testutil.MustApply(
			ctx, t,
			insert.InvocationWithInclusions("i0", pb.Invocation_ACTIVE, map[string]any{
				"Realm":        "testproject:testrealm",
				"Sources":      spanutil.Compress(pbutil.MustMarshal(testutil.TestSources())),
				"Instructions": spanutil.Compress(pbutil.MustMarshal(testInstructions)),
			}, "i1", "missinginv")...,
		)
		testutil.MustApply(
			ctx, t,
			insert.Invocation("i1", pb.Invocation_ACTIVE, map[string]any{
				"Realm": "testproject:testrealm",
			}),
		)
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.TestResults(t, "i0", "test1", pbutil.Variant("a", "b"), pb.TestResult_SKIPPED),
			insert.TestResults(t, "i0", "test2", pbutil.Variant("c", "d"), pb.TestResult_PASSED),
			insert.TestResults(t, "i0", "test3", pbutil.Variant("a", "b"), pb.TestResult_FAILED),
			insert.TestResults(t, "i0", "test4", pbutil.Variant("g", "h"), pb.TestResult_EXECUTION_ERRORED),
			insert.TestResults(t, "i0", "test5", pbutil.Variant(), pb.TestResult_PRECLUDED),
			insert.TestResults(t, "i1", "test1", pbutil.Variant("e", "f"), pb.TestResult_PASSED),
			insert.TestResults(t, "i1", "test3", pbutil.Variant("c", "d"), pb.TestResult_PASSED),
		)...)

		// Set up root invocation, work units and their test results.
		// root-inv1 -> root wu -> wu1
		// root-inv2 -> root wu
		rootInv := rootinvocations.NewBuilder("root-inv1").WithState(pb.RootInvocation_ACTIVE).WithRealm("testproject:rootrealm").Build()
		rootWU := workunits.NewBuilder(rootInv.RootInvocationID, "root").WithState(pb.WorkUnit_ACTIVE).WithRealm("testproject:wurealm").Build()
		wu1 := workunits.
			NewBuilder(rootInv.RootInvocationID, "wu1").
			WithState(pb.WorkUnit_ACTIVE).
			WithRealm("testproject:wurealm").
			WithInstructions(testInstructions).
			Build()
		tr11 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test1", pbutil.Variant("a", "b"), pb.TestResult_SKIPPED)[0]
		tr12 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test2", pbutil.Variant("c", "d"), pb.TestResult_PASSED)[0]
		tr13 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test3", pbutil.Variant("a", "b"), pb.TestResult_FAILED)[0]
		tr14 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test4", pbutil.Variant("g", "h"), pb.TestResult_EXECUTION_ERRORED)[0]
		tr15 := insert.MakeTestResults(wu1.ID.LegacyInvocationID(), "test5", pbutil.Variant(), pb.TestResult_PRECLUDED)[0]

		rootInv2 := rootinvocations.NewBuilder("root-inv2").WithState(pb.RootInvocation_ACTIVE).WithRealm("testproject:rootrealm").Build()
		rootWU2 := workunits.NewBuilder(rootInv2.RootInvocationID, "root").WithState(pb.WorkUnit_ACTIVE).WithRealm("testproject:wurealm").Build()
		tr21 := insert.MakeTestResults(rootWU2.ID.LegacyInvocationID(), "test1", pbutil.Variant("e", "f"), pb.TestResult_PASSED)[0]
		tr23 := insert.MakeTestResults(rootWU2.ID.LegacyInvocationID(), "test3", pbutil.Variant("c", "d"), pb.TestResult_PASSED)[0]
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			rootinvocations.InsertForTesting(rootInv),
			rootinvocations.InsertForTesting(rootInv2),
			workunits.InsertForTesting(rootWU),
			workunits.InsertForTesting(rootWU2),
			workunits.InsertForTesting(wu1),
			insert.TestResultMessages(t, []*pb.TestResult{tr11, tr12, tr13, tr14, tr15, tr21, tr23}),
		)...)

		srv := newTestResultDBService()

		req := &pb.BatchGetTestVariantsRequest{
			Parent: "rootInvocations/root-inv1",
			TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
				{TestId: "test1", VariantHash: variantHash("a", "b")},
				{TestId: "test1", VariantHash: variantHash("g", "h")}, // test variant doesn't exist, verify combines test ID and variant hash correctly.
				{TestId: "test1", VariantHash: variantHash("e", "f")}, // Exist, but in another root invocation.
				{TestId: "test3", VariantHash: variantHash("a", "b")},
				{TestId: "test3", VariantHash: variantHash("g", "h")}, // test variant doesn't exist, verify combines test ID and variant hash correctly.
				{TestId: "test3", VariantHash: variantHash("c", "d")}, // Exist, but in another root invocation.
				{TestId: "test4", VariantHash: variantHash("g", "h")},
				{TestId: "test4", VariantHash: variantHash("a", "b")}, // test variant doesn't exist, verify combines test ID and variant hash correctly.
			},
		}
		// Expected results from the request above.
		expectedResults := []*pb.TestResult{tr13, tr14, tr11}
		expectedResultsLegacy := []*pb.TestResult{
			insert.MakeTestResults("i0", "test3", pbutil.Variant("a", "b"), pb.TestResult_FAILED)[0],
			insert.MakeTestResults("i0", "test4", pbutil.Variant("g", "h"), pb.TestResult_EXECUTION_ERRORED)[0],
			insert.MakeTestResults("i0", "test1", pbutil.Variant("a", "b"), pb.TestResult_SKIPPED)[0],
		}
		expectedStatus := []struct {
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
				Status:         pb.TestVariantStatus_EXPECTED,
				StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
				StatusV2:       pb.TestVerdict_SKIPPED,
			},
		}

		t.Run(`Request validation`, func(t *ftt.Test) {
			t.Run(`invalid invocation name`, func(t *ftt.Test) {
				req.Parent = ""
				req.Invocation = "rootInvocations/i0"
				_, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("does not match pattern"))
			})

			t.Run(`invalid parent name`, func(t *ftt.Test) {
				req.Parent = "invocations/i0"
				_, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("does not match pattern"))
			})

			t.Run(`parent name and invocation name both exist`, func(t *ftt.Test) {
				req.Invocation = "invocations/i0"
				req.Parent = "rootInvocations/i0"
				_, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`invocation: must not be specified if parent is specified`))
			})

			t.Run(`parent name and invocation name both not exist`, func(t *ftt.Test) {
				req.Invocation = ""
				req.Parent = ""
				_, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("either parent or invocation must be specified"))
			})

			// Other request validation is tested in TestValidateBatchGetTestVariantsRequest. Here we just make sure the function is called.
			t.Run(`invalid test id`, func(t *ftt.Test) {
				req.TestVariants[0].TestId = "\x00"
				_, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`test_id: non-printable rune`))
			})
		})

		t.Run(`Access denied`, func(t *ftt.Test) {
			t.Run(`legacy invocation`, func(t *ftt.Test) {
				req.Invocation = "invocations/i0"
				req.Parent = ""
				t.Run(`missing ListTestResults permission`, func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: "user:someone@example.com",
						IdentityPermissions: []authtest.RealmPermission{
							{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
						},
					})

					_, err := srv.BatchGetTestVariants(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission resultdb.testResults.list in realm of "invocations/i0"`))
				})

				t.Run(`missing ListTestExonerations permission`, func(t *ftt.Test) {
					ctx := auth.WithState(ctx, &authtest.FakeState{
						Identity: "user:someone@example.com",
						IdentityPermissions: []authtest.RealmPermission{
							{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
						},
					})

					_, err := srv.BatchGetTestVariants(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission resultdb.testExonerations.list in realm of "invocations/i0"`))
				})
			})

			t.Run(`parent`, func(t *ftt.Test) {
				req.Invocation = ""
				req.Parent = "rootInvocations/root-inv1"
				t.Run(`missing ListLimitedTestResults permission`, func(t *ftt.Test) {
					ctx := auth.WithState(ctx, &authtest.FakeState{
						Identity: "user:someone@example.com",
						IdentityPermissions: []authtest.RealmPermission{
							{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedTestExonerations},
						},
					})

					_, err := srv.BatchGetTestVariants(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission resultdb.testResults.listLimited in realm of "rootInvocations/root-inv1"`))
				})

				t.Run(`missing ListLimitedTestExonerations permission`, func(t *ftt.Test) {
					ctx := auth.WithState(ctx, &authtest.FakeState{
						Identity: "user:someone@example.com",
						IdentityPermissions: []authtest.RealmPermission{
							{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedTestResults},
						},
					})

					_, err := srv.BatchGetTestVariants(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission resultdb.testExonerations.listLimited in realm of "rootInvocations/root-inv1"`))
				})
			})
		})
		t.Run("not found", func(t *ftt.Test) {
			t.Run("parent", func(t *ftt.Test) {
				req.Invocation = ""
				req.Parent = "rootInvocations/missingroot"

				_, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`"rootInvocations/missingroot" not found`))
			})
			t.Run("invocation (legacy)", func(t *ftt.Test) {
				req.Invocation = "invocations/missinginv"
				req.Parent = ""

				_, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`invocations/missinginv not found`))
			})
		})
		t.Run("e2e", func(t *ftt.Test) {
			req.Invocation = ""
			req.Parent = "rootInvocations/root-inv1"

			t.Run(`empty test variants`, func(t *ftt.Test) {
				req.TestVariants = []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{}
				res, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.BatchGetTestVariantsResponse{}))
			})

			t.Run(`baseline`, func(t *ftt.Test) {
				res, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				for i, tv := range res.TestVariants {
					result := expectedResults[i]

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
						Status:           expectedStatus[i].Status,
						StatusOverride:   expectedStatus[i].StatusOverride,
						StatusV2:         expectedStatus[i].StatusV2,
						Results:          []*pb.TestResultBundle{{Result: expectedResult}},
						TestMetadata:     result.TestMetadata,
						Instruction:      &pb.VerdictInstruction{Instruction: "rootInvocations/root-inv1/workUnits/wu1/instructions/test"},
						SourcesId:        graph.HashSources(rootInv.Sources).String(),
					}))
				}

				// Check Sources field.
				assert.Loosely(t, res.Sources, should.HaveLength(1))
				assert.Loosely(t, res.Sources[graph.HashSources(rootInv.Sources).String()], should.Match(rootInv.Sources))
			})

			t.Run(`Limited access`, func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedTestResults},
						{Realm: "testproject:rootrealm", Permission: rdbperms.PermListLimitedTestExonerations},
					},
				})

				res, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				for i, tv := range res.TestVariants {
					result := expectedResults[i]

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
						Status:         expectedStatus[i].Status,
						StatusOverride: expectedStatus[i].StatusOverride,
						StatusV2:       expectedStatus[i].StatusV2,
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
			})

			t.Run(`Valid request with structured test IDs`, func(t *ftt.Test) {
				req.TestVariants = []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestIdStructured: &pb.TestIdentifier{
						ModuleName:        "legacy",
						ModuleScheme:      "legacy",
						ModuleVariantHash: variantHash("a", "b"),
						CaseName:          "test1",
					}},
					{TestIdStructured: &pb.TestIdentifier{
						ModuleName:    "legacy",
						ModuleScheme:  "legacy",
						ModuleVariant: pbutil.Variant("a", "b"),
						CaseName:      "test3",
					}},
					{TestIdStructured: &pb.TestIdentifier{
						ModuleName:        "legacy",
						ModuleScheme:      "legacy",
						ModuleVariant:     pbutil.Variant("g", "h"),
						ModuleVariantHash: variantHash("g", "h"),
						CaseName:          "test4",
					}},
					{TestIdStructured: &pb.TestIdentifier{
						ModuleName:    "legacy",
						ModuleScheme:  "legacy",
						ModuleVariant: &pb.Variant{},
						CaseName:      "test5",
					}},
				}

				res, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				// NOTE: The order isn't important here, we just don't have a
				// matcher that does an unordered comparison.
				assert.Loosely(t, tvStrings(res.TestVariants), should.Match([]string{
					fmt.Sprintf("10/test3/%s/FAILED", variantHash("a", "b")),
					fmt.Sprintf("20/test4/%s/EXECUTION_ERRORED", variantHash("g", "h")),
					fmt.Sprintf("20/test5/%s/PRECLUDED", variantHash()),
					fmt.Sprintf("50/test1/%s/SKIPPED", variantHash("a", "b")),
				}))
			})
		})

		t.Run("e2e legacy", func(t *ftt.Test) {
			req.Invocation = "invocations/i0"
			req.Parent = ""
			t.Run("baseline", func(t *ftt.Test) {
				req.TestVariants = []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					{TestId: "test1", VariantHash: variantHash("a", "b")},
					{TestId: "test3", VariantHash: variantHash("a", "b")},
					{TestId: "test4", VariantHash: variantHash("g", "h")},
				}

				res, err := srv.BatchGetTestVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				for i, tv := range res.TestVariants {
					result := expectedResultsLegacy[i]
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
						Status:           expectedStatus[i].Status,
						StatusOverride:   expectedStatus[i].StatusOverride,
						StatusV2:         expectedStatus[i].StatusV2,
						Results:          []*pb.TestResultBundle{{Result: expectedResult}},
						TestMetadata:     result.TestMetadata,
						Instruction:      &pb.VerdictInstruction{Instruction: "invocations/i0/instructions/test"},
						SourcesId:        graph.HashSources(testutil.TestSources()).String(),
					}))
				}

				// Check Sources field.
				assert.Loosely(t, res.Sources, should.HaveLength(1))
				assert.Loosely(t, res.Sources[graph.HashSources(testutil.TestSources()).String()], should.Match(testutil.TestSources()))
			})

			t.Run(`Valid request without included invocation`, func(t *ftt.Test) {
				res, err := srv.BatchGetTestVariants(ctx, &pb.BatchGetTestVariantsRequest{
					Invocation: "invocations/i1",
					TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
						{TestId: "test1", VariantHash: variantHash("e", "f")},
						{TestId: "test3", VariantHash: variantHash("c", "d")},
					},
				})
				assert.Loosely(t, err, should.BeNil)

				// NOTE: The order isn't important here, we just don't have a
				// matcher that does an unordered comparison.
				assert.Loosely(t, tvStrings(res.TestVariants), should.Match([]string{
					fmt.Sprintf("50/test1/%s/PASSED", variantHash("e", "f")),
					fmt.Sprintf("50/test3/%s/PASSED", variantHash("c", "d")),
				}))

				for _, tv := range res.TestVariants {
					assert.Loosely(t, tv.IsMasked, should.BeFalse)
					assert.Loosely(t, tv.SourcesId, should.BeEmpty)
				}
				assert.Loosely(t, res.Sources, should.HaveLength(0))
			})
		})
	})
}

func TestValidateBatchGetTestVariantsRequest(t *testing.T) {
	ftt.Run(`validateBatchGetTestVariantsRequest`, t, func(t *ftt.Test) {
		request := &pb.BatchGetTestVariantsRequest{
			Invocation: "invocations/i0",
			TestVariants: []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
				{TestId: "test1", VariantHash: variantHash("a", "b")},
			},
			ResultLimit: 10,
		}
		t.Run(`base case`, func(t *ftt.Test) {
			err := validateBatchGetTestVariantsRequest(request)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`negative result_limit`, func(t *ftt.Test) {
			request.ResultLimit = -1
			err := validateBatchGetTestVariantsRequest(request)
			assert.Loosely(t, err, should.ErrLike(`result_limit: negative`))
		})
		t.Run(`structured test ID`, func(t *ftt.Test) {
			request.TestVariants = []*pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
				{TestIdStructured: &pb.TestIdentifier{
					ModuleName:        "module",
					ModuleScheme:      "junit",
					ModuleVariantHash: variantHash("a", "b"),
					CoarseName:        "package",
					FineName:          "class",
					CaseName:          "method",
				}},
			}
			t.Run(`base case`, func(t *ftt.Test) {
				err := validateBatchGetTestVariantsRequest(request)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`invalid structured ID`, func(t *ftt.Test) {
				request.TestVariants[0].TestIdStructured.ModuleName = ""
				err := validateBatchGetTestVariantsRequest(request)
				assert.Loosely(t, err, should.ErrLike("test_variants[0]: test_id_structured: module_name: unspecified"))
			})
			// It is not valid to also specify a flat test ID.
			t.Run(`flat test ID specified`, func(t *ftt.Test) {
				request.TestVariants[0].TestId = "blah"
				err := validateBatchGetTestVariantsRequest(request)
				assert.Loosely(t, err, should.ErrLike("test_variants[0]: test_id: may not be set at same time as test_id_structured"))
			})
			t.Run(`flat variant hash specified`, func(t *ftt.Test) {
				request.TestVariants[0].VariantHash = "blah"
				err := validateBatchGetTestVariantsRequest(request)
				assert.Loosely(t, err, should.ErrLike("test_variants[0]: variant_hash: may not be set at same time as test_id_structured"))
			})
		})
		t.Run(`flat test ID`, func(t *ftt.Test) {
			t.Run(`test ID invalid`, func(t *ftt.Test) {
				request.TestVariants[0].TestId = "\x00"
				err := validateBatchGetTestVariantsRequest(request)
				assert.Loosely(t, err, should.ErrLike("test_variants[0]: test_id: non-printable rune"))
			})
			t.Run(`variant hash invalid`, func(t *ftt.Test) {
				request.TestVariants[0].VariantHash = ""
				err := validateBatchGetTestVariantsRequest(request)
				assert.Loosely(t, err, should.ErrLike("test_variants[0]: variant_hash: unspecified"))
			})
		})
		t.Run(`>= 500 test variants`, func(t *ftt.Test) {
			req := &pb.BatchGetTestVariantsRequest{
				Invocation:   "invocations/i0",
				TestVariants: make([]*pb.BatchGetTestVariantsRequest_TestVariantIdentifier, 501),
			}
			for i := 0; i < 500; i += 1 {
				req.TestVariants[i] = &pb.BatchGetTestVariantsRequest_TestVariantIdentifier{
					TestId:      "test1",
					VariantHash: variantHash("a", "b"),
				}
			}

			err := validateBatchGetTestVariantsRequest(req)
			assert.Loosely(t, err, should.ErrLike(`a maximum of 500 test variants can be requested at once`))
		})
	})
}
