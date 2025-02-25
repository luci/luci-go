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
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/testvariants"
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
			insert.TestResults(t, "inv0", "T1", nil, pb.TestStatus_FAIL),
			insert.TestResults(t, "inv0", "T2", nil, pb.TestStatus_FAIL),
			insert.TestResults(t, "inv1", "T3", nil, pb.TestStatus_PASS),
			insert.TestResults(t, "inv1", "T1", pbutil.Variant("a", "b"), pb.TestStatus_FAIL, pb.TestStatus_PASS),
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
			insert.TestResults(t, "inv2", "T1002", pbutil.Variant("k0", "v0"), pb.TestStatus_FAIL),
			insert.TestResults(t, "inv3", "T1003", pbutil.Variant("k1", "v1"), pb.TestStatus_FAIL),
			insert.TestResults(t, "inv4", "T1004", pbutil.Variant("k2", "v2"), pb.TestStatus_FAIL),
			insert.TestResults(t, "inv5", "T1005", pbutil.Variant("k3", "v3"), pb.TestStatus_FAIL),
			insert.TestExonerations("inv3", "T1003", pbutil.Variant("k1", "v1"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
			insert.TestExonerations("inv4", "T1004", pbutil.Variant("k2", "v2"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
			insert.TestExonerations("inv5", "T1005", pbutil.Variant("k3", "v3"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
		)...)

		srv := newTestResultDBService()

		getTVStrings := func(tvs []*pb.TestVariant) []string {
			tvStrings := make([]string, len(tvs))
			for i, tv := range tvs {
				tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
			}
			return tvStrings
		}

		t.Run(`Permission denied`, func(t *ftt.Test) {
			req := &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0"},
			}
			// Test PermListLimitedTestResults is required if the user does not have
			// both PermListTestResults and PermListTestExonerations.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestExonerations},
				},
			})
			_, err := srv.QueryTestVariants(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.testResults.listLimited"))

			// Test PermListLimitedTestExonerations is required if the user does not
			// have both PermListTestResults and PermListTestExonerations.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestResults},
				},
			})
			_, err = srv.QueryTestVariants(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.testExonerations.listLimited"))
		})

		t.Run(`Valid with limited list permission`, func(t *ftt.Test) {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv2"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.TestVariants), should.Equal(4))

			// Check the returned test variants are appropriately masked.
			duration := &durationpb.Duration{Seconds: 0, Nanos: 234567000}
			assert.Loosely(t, res.TestVariants, should.Match([]*pb.TestVariant{
				{
					TestId:      "T1002",
					VariantHash: pbutil.VariantHash(pbutil.Variant("k0", "v0")),
					Status:      pb.TestVariantStatus_UNEXPECTED,
					Results: []*pb.TestResultBundle{
						{
							Result: &pb.TestResult{
								Name:     "invocations/inv2/tests/T1002/results/0",
								ResultId: "0",
								Status:   pb.TestStatus_FAIL,
								Duration: duration,
								FailureReason: &pb.FailureReason{
									PrimaryErrorMessage: "failure reason",
								},
								IsMasked: true,
							},
						},
					},
					IsMasked:  true,
					SourcesId: graph.HashSources(testutil.TestSourcesWithChangelistNumbers(2)).String(),
				},
				{
					TestId:      "T1003",
					Variant:     pbutil.Variant("k1", "v1"),
					VariantHash: pbutil.VariantHash(pbutil.Variant("k1", "v1")),
					Status:      pb.TestVariantStatus_EXONERATED,
					Results: []*pb.TestResultBundle{
						{
							Result: &pb.TestResult{
								Name:        "invocations/inv3/tests/T1003/results/0",
								ResultId:    "0",
								Status:      pb.TestStatus_FAIL,
								Duration:    duration,
								SummaryHtml: "SummaryHtml",
								FailureReason: &pb.FailureReason{
									PrimaryErrorMessage: "failure reason",
								},
								Properties: &structpb.Struct{Fields: map[string]*structpb.Value{
									"key": structpb.NewStringValue("value"),
								}},
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
					TestId:      "T1004",
					Variant:     pbutil.Variant("k2", "v2"),
					VariantHash: pbutil.VariantHash(pbutil.Variant("k2", "v2")),
					Status:      pb.TestVariantStatus_EXONERATED,
					Results: []*pb.TestResultBundle{
						{
							Result: &pb.TestResult{
								Name:     "invocations/inv4/tests/T1004/results/0",
								ResultId: "0",
								Status:   pb.TestStatus_FAIL,
								Duration: duration,
								FailureReason: &pb.FailureReason{
									PrimaryErrorMessage: "failure reason",
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
					TestId:      "T1005",
					VariantHash: pbutil.VariantHash(pbutil.Variant("k3", "v3")),
					Status:      pb.TestVariantStatus_EXONERATED,
					Results: []*pb.TestResultBundle{
						{
							Result: &pb.TestResult{
								Name:     "invocations/inv5/tests/T1005/results/0",
								ResultId: "0",
								Status:   pb.TestStatus_FAIL,
								FailureReason: &pb.FailureReason{
									PrimaryErrorMessage: "failure reason",
								},
								Duration: duration,
								IsMasked: true,
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

		t.Run(`Valid with included invocation`, func(t *ftt.Test) {
			page, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, page.NextPageToken, should.Equal(pagination.Token("EXPECTED", "", "")))

			assert.Loosely(t, len(page.TestVariants), should.Equal(3))
			assert.Loosely(t, getTVStrings(page.TestVariants), should.Match([]string{
				"10/T2/e3b0c44298fc1c14",
				"30/T1/c467ccce5a16dc72",
				"40/T1/e3b0c44298fc1c14",
			}))

			expectedSources := testutil.TestSourcesWithChangelistNumbers(1)
			expectedSourceHash := graph.HashSources(expectedSources).String()
			for _, tv := range page.TestVariants {
				assert.Loosely(t, tv.SourcesId, should.Equal(expectedSourceHash))
			}

			assert.Loosely(t, page.Sources, should.HaveLength(1))
			assert.Loosely(t, page.Sources[expectedSourceHash], should.Match(expectedSources))
		})

		t.Run(`Valid without included invocation`, func(t *ftt.Test) {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv1"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.NextPageToken, should.Equal(pagination.Token("EXPECTED", "", "")))

			assert.Loosely(t, len(res.TestVariants), should.Equal(1))
			assert.Loosely(t, getTVStrings(res.TestVariants), should.Match([]string{
				"30/T1/c467ccce5a16dc72",
			}))
		})

		t.Run(`Too many invocations`, func(t *ftt.Test) {
			_, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0", "invocations/inv1"},
				PageSize:    1,
			})

			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invocations: only one invocation is allowed"))
		})

		t.Run(`Try next page`, func(t *ftt.Test) {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0"},
				PageSize:    3,
				PageToken:   pagination.Token("EXPECTED", "", ""),
			})

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.TestVariants), should.Equal(1))
		})
	})
}

func TestValidateQueryTestVariantsRequest(t *testing.T) {
	ftt.Run(`validateQueryTestVariantsRequest`, t, func(t *ftt.Test) {
		t.Run(`negative result_limit`, func(t *ftt.Test) {
			err := validateQueryTestVariantsRequest(&pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/invx"},
				ResultLimit: -1,
			})
			assert.Loosely(t, err, should.ErrLike(`result_limit: negative`))
		})
	})
}

func TestDetermineListAccessLevel(t *testing.T) {
	ftt.Run("determineListAccessLevel", t, func(t *ftt.Test) {
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

		t.Run("Access denied", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i0"), invocations.ID("i2"))
			accessLevel, err := determineListAccessLevel(ctx, ids)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelInvalid))
		})
		t.Run("No common access level", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i3"))
			accessLevel, err := determineListAccessLevel(ctx, ids)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelInvalid))
		})
		t.Run("Limited access", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i2b"),
				invocations.ID("i3"), invocations.ID("i3b"))
			accessLevel, err := determineListAccessLevel(ctx, ids)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelLimited))
		})
		t.Run("Full access", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i2"),
				invocations.ID("i2b"))
			accessLevel, err := determineListAccessLevel(ctx, ids)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelUnrestricted))
		})
		t.Run("No invocations", func(t *ftt.Test) {
			accessLevel, err := determineListAccessLevel(ctx, invocations.NewIDSet())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, accessLevel, should.Equal(testvariants.AccessLevelInvalid))
		})
	})
}
