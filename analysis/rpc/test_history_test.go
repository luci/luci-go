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

package rpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/testrealms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestTestHistoryServer(t *testing.T) {
	ftt.Run("TestHistoryServer", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{
					Realm:      "project:realm",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project:realm",
					Permission: rdbperms.PermListTestExonerations,
				},
				{
					Realm:      "project:other-realm",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project:other-realm",
					Permission: rdbperms.PermListTestExonerations,
				},
			},
		})

		referenceTime := time.Date(2025, time.February, 12, 0, 0, 0, 0, time.UTC)
		day := 24 * time.Hour

		var1 := pbutil.Variant("key1", "val1", "key2", "val1")
		var2 := pbutil.Variant("key1", "val2", "key2", "val1")
		var3 := pbutil.Variant("key1", "val2", "key2", "val2")
		var4 := pbutil.Variant("key1", "val1", "key2", "val2")
		var5 := pbutil.Variant("key1", "val3", "key2", "val2")

		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			insertTVR := func(testID, subRealm string, variant *pb.Variant) {
				span.BufferWrite(ctx, (&testresults.TestVariantRealm{
					Project:     "project",
					TestID:      testID,
					SubRealm:    subRealm,
					Variant:     variant,
					VariantHash: pbutil.VariantHash(variant),
				}).SaveUnverified())
			}

			insertTVR("test_id", "realm", var1)
			insertTVR("previous_test_id", "realm", var1)
			insertTVR("test_id", "realm", var2)
			insertTVR("previous_test_id", "realm", var3)
			insertTVR("test_id", "other-realm", var4)
			insertTVR("test_id", "forbidden-realm", var5)

			insertTV := func(partitionTime time.Time, testID string, variant *pb.Variant, invId string, hasUnsubmittedChanges bool, isFromBisection bool, subRealm string) {
				baseTestResult := testresults.NewTestResult().
					WithProject("project").
					WithTestID(testID).
					WithVariantHash(pbutil.VariantHash(variant)).
					WithPartitionTime(partitionTime).
					WithIngestedInvocationID(invId).
					WithSubRealm(subRealm).
					WithStatus(pb.TestResultStatus_PASS).
					WithStatusV2(pb.TestResult_PASSED).
					WithIsFromBisection(isFromBisection).
					WithoutRunDuration()
				if hasUnsubmittedChanges {
					baseTestResult = baseTestResult.WithSources(testresults.Sources{
						Changelists: []testresults.Changelist{
							{
								Host:      "anothergerrit.gerrit.instance",
								Change:    5471,
								Patchset:  6,
								OwnerKind: pb.ChangelistOwnerKind_HUMAN,
							},
							{
								Host:      "mygerrit-review.googlesource.com",
								Change:    4321,
								Patchset:  5,
								OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
							},
						},
					})
				} else {
					baseTestResult = baseTestResult.WithSources(testresults.Sources{})
				}

				trs := testresults.NewTestVerdict().
					WithBaseTestResult(baseTestResult.Build()).
					WithStatus(pb.TestVerdict_PASSED).
					WithPassedAvgDuration(nil).
					Build()
				for _, tr := range trs {
					span.BufferWrite(ctx, tr.SaveUnverified())
				}
			}

			insertTV(referenceTime.Add(-1*day), "test_id", var1, "inv1", false, false, "realm")
			insertTV(referenceTime.Add(-1*day), "test_id", var1, "inv2", false, false, "realm")
			insertTV(referenceTime.Add(-1*day), "test_id", var2, "inv1", false, false, "realm")
			insertTV(referenceTime.Add(-1*day), "test_id", var2, "inv2", false, true, "realm")

			insertTV(referenceTime.Add(-2*day), "previous_test_id", var1, "inv1", false, false, "realm")
			insertTV(referenceTime.Add(-2*day), "test_id", var1, "inv2", true, false, "realm")
			insertTV(referenceTime.Add(-2*day), "test_id", var2, "inv1", true, false, "realm")

			insertTV(referenceTime.Add(-3*day), "previous_test_id", var3, "inv1", true, false, "realm")

			insertTV(referenceTime.Add(-4*day), "test_id", var4, "inv2", false, false, "other-realm")
			insertTV(referenceTime.Add(-5*day), "test_id", var5, "inv3", false, false, "forbidden-realm")

			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		searchClient := &testrealms.FakeClient{}
		server := NewTestHistoryServer(searchClient)

		// Install a fake ResultDB test metadata client in the context.
		fakeRDBClient := &resultdb.FakeClient{
			TestMetadata: []*resultpb.TestMetadataDetail{
				{
					Project: "project",
					TestId:  "test_id",
					SourceRef: &resultpb.SourceRef{
						System: &resultpb.SourceRef_Gitiles{
							Gitiles: &resultpb.GitilesRef{
								Host: "chromium.googlesource.com",
								Ref:  "refs/heads/other",
							},
						},
					},
				},
				{
					Project: "project",
					TestId:  "test_id",
					SourceRef: &resultpb.SourceRef{
						System: &resultpb.SourceRef_Gitiles{
							Gitiles: &resultpb.GitilesRef{
								Host: "chromium.googlesource.com",
								Ref:  "refs/heads/main",
							},
						},
					},
					TestMetadata: &resultpb.TestMetadata{
						PreviousTestId: "previous_test_id",
					},
				},
			},
		}
		ctx = resultdb.UseClientForTesting(ctx, fakeRDBClient)

		t.Run("Query", func(t *ftt.Test) {
			req := &pb.QueryTestHistoryRequest{
				Project: "project",
				TestId:  "test_id",
				Predicate: &pb.TestVerdictPredicate{
					SubRealm: "realm",
				},
				FollowTestIdRenaming: true,
				PageSize:             5,
			}

			expectedChangelists := []*pb.Changelist{
				{
					Host:      "anothergerrit.gerrit.instance",
					Change:    5471,
					Patchset:  6,
					OwnerKind: pb.ChangelistOwnerKind_HUMAN,
				},
				{
					Host:      "mygerrit-review.googlesource.com",
					Change:    4321,
					Patchset:  5,
					OwnerKind: pb.ChangelistOwnerKind_AUTOMATION,
				},
			}

			t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
				testPerm := func(ctx context.Context) {
					res, err := server.Query(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission`))
					assert.Loosely(t, err, should.ErrLike(`in realm "project:realm"`))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, res, should.BeNil)
				}

				// No permission.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				testPerm(ctx)

				// testResults.list only.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{
							Realm:      "project:realm",
							Permission: rdbperms.PermListTestResults,
						},
						{
							Realm:      "project:other_realm",
							Permission: rdbperms.PermListTestExonerations,
						},
					},
				})
				testPerm(ctx)

				// testExonerations.list only.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{
							Realm:      "project:other_realm",
							Permission: rdbperms.PermListTestResults,
						},
						{
							Realm:      "project:realm",
							Permission: rdbperms.PermListTestExonerations,
						},
					},
				})
				testPerm(ctx)
			})

			t.Run("invalid requests are rejected", func(t *ftt.Test) {
				req.PageSize = -1
				res, err := server.Query(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("multi-realms", func(t *ftt.Test) {
				req.Predicate.SubRealm = ""
				req.Predicate.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key2", "val2"),
					},
				}
				res, err := server.Query(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestHistoryResponse{
					Verdicts: []*pb.TestVerdict{
						{
							TestId:         "previous_test_id",
							VariantHash:    pbutil.VariantHash(var3),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-3 * day)),
							Changelists:    expectedChangelists,
						},
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var4),
							InvocationId:   "inv2",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-4 * day)),
						},
					},
				}))
			})

			t.Run("e2e", func(t *ftt.Test) {
				res, err := server.Query(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestHistoryResponse{
					Verdicts: []*pb.TestVerdict{
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var1),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var1),
							InvocationId:   "inv2",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var2),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:         "previous_test_id",
							VariantHash:    pbutil.VariantHash(var1),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-2 * day)),
						},
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var1),
							InvocationId:   "inv2",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-2 * day)),
							Changelists:    expectedChangelists,
						},
					},
					NextPageToken: res.NextPageToken,
				}))
				assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = server.Query(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestHistoryResponse{
					Verdicts: []*pb.TestVerdict{
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var2),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-2 * day)),
							Changelists:    expectedChangelists,
						},
						{
							TestId:         "previous_test_id",
							VariantHash:    pbutil.VariantHash(var3),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-3 * day)),
							Changelists:    expectedChangelists,
						},
					},
				}))
			})
			t.Run("without follow renames", func(t *ftt.Test) {
				req.FollowTestIdRenaming = false
				req.PageSize = 10
				res, err := server.Query(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				// Two less than the baseline test.
				assert.Loosely(t, len(res.Verdicts), should.Equal(5))
			})

			t.Run("include bisection", func(t *ftt.Test) {
				req.PageSize = 10
				req.Predicate.IncludeBisectionResults = true
				res, err := server.Query(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestHistoryResponse{
					Verdicts: []*pb.TestVerdict{
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var1),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var1),
							InvocationId:   "inv2",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var2),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var2),
							InvocationId:   "inv2",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-1 * day)),
						},
						{
							TestId:         "previous_test_id",
							VariantHash:    pbutil.VariantHash(var1),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-2 * day)),
						},
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var1),
							InvocationId:   "inv2",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-2 * day)),
							Changelists:    expectedChangelists,
						},
						{
							TestId:         "test_id",
							VariantHash:    pbutil.VariantHash(var2),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-2 * day)),
							Changelists:    expectedChangelists,
						},
						{
							TestId:         "previous_test_id",
							VariantHash:    pbutil.VariantHash(var3),
							InvocationId:   "inv1",
							Status:         pb.TestVerdictStatus_EXPECTED,
							StatusV2:       pb.TestVerdict_PASSED,
							StatusOverride: pb.TestVerdict_NOT_OVERRIDDEN,
							PartitionTime:  timestamppb.New(referenceTime.Add(-3 * day)),
							Changelists:    expectedChangelists,
						},
					},
				}))
			})
		})

		t.Run("QueryStats", func(t *ftt.Test) {
			req := &pb.QueryTestHistoryStatsRequest{
				Project: "project",
				TestId:  "test_id",
				Predicate: &pb.TestVerdictPredicate{
					SubRealm: "realm",
				},
				FollowTestIdRenaming: true,
				PageSize:             3,
			}

			t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
				testPerm := func(ctx context.Context) {
					res, err := server.QueryStats(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission`))
					assert.Loosely(t, err, should.ErrLike(`in realm "project:realm"`))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, res, should.BeNil)
				}

				// No permission.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				testPerm(ctx)

				// testResults.list only.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{
							Realm:      "project:realm",
							Permission: rdbperms.PermListTestResults,
						},
						{
							Realm:      "project:other_realm",
							Permission: rdbperms.PermListTestExonerations,
						},
					},
				})
				testPerm(ctx)

				// testExonerations.list only.
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{
							Realm:      "project:other_realm",
							Permission: rdbperms.PermListTestResults,
						},
						{
							Realm:      "project:realm",
							Permission: rdbperms.PermListTestExonerations,
						},
					},
				})
				testPerm(ctx)
			})

			t.Run("invalid requests are rejected", func(t *ftt.Test) {
				req.PageSize = -1
				res, err := server.QueryStats(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("multi-realms", func(t *ftt.Test) {
				req.Predicate.SubRealm = ""
				req.Predicate.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key2", "val2"),
					},
				}
				res, err := server.QueryStats(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestHistoryStatsResponse{
					Groups: []*pb.QueryTestHistoryStatsResponse_Group{
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-3 * day)),
							VariantHash:   pbutil.VariantHash(var3),
							ExpectedCount: 1,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 1,
							},
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-4 * day)),
							VariantHash:   pbutil.VariantHash(var4),
							ExpectedCount: 1,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 1,
							},
						},
					},
				}))
			})

			t.Run("e2e", func(t *ftt.Test) {
				res, err := server.QueryStats(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestHistoryStatsResponse{
					Groups: []*pb.QueryTestHistoryStatsResponse_Group{
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
							VariantHash:   pbutil.VariantHash(var1),
							ExpectedCount: 2,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 2,
							},
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
							VariantHash:   pbutil.VariantHash(var2),
							ExpectedCount: 1,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 1,
							},
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							VariantHash:   pbutil.VariantHash(var1),
							ExpectedCount: 2,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 2,
							},
						},
					},
					NextPageToken: res.NextPageToken,
				}))
				assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = server.QueryStats(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestHistoryStatsResponse{
					Groups: []*pb.QueryTestHistoryStatsResponse_Group{
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							VariantHash:   pbutil.VariantHash(var2),
							ExpectedCount: 1,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 1,
							},
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-3 * day)),
							VariantHash:   pbutil.VariantHash(var3),
							ExpectedCount: 1,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 1,
							},
						},
					},
				}))
			})
			t.Run("without follow renames", func(t *ftt.Test) {
				req.FollowTestIdRenaming = false
				req.PageSize = 10
				res, err := server.QueryStats(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				// One less than the baseline test.
				assert.Loosely(t, len(res.Groups), should.Equal(4))
			})

			t.Run("include bisection", func(t *ftt.Test) {
				req.Predicate.IncludeBisectionResults = true
				req.PageSize = 10
				res, err := server.QueryStats(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestHistoryStatsResponse{
					Groups: []*pb.QueryTestHistoryStatsResponse_Group{
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
							VariantHash:   pbutil.VariantHash(var1),
							ExpectedCount: 2,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 2,
							},
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-1 * day)),
							VariantHash:   pbutil.VariantHash(var2),
							ExpectedCount: 2,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 2,
							},
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							VariantHash:   pbutil.VariantHash(var1),
							ExpectedCount: 2,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 2,
							},
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-2 * day)),
							VariantHash:   pbutil.VariantHash(var2),
							ExpectedCount: 1,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 1,
							},
						},
						{
							PartitionTime: timestamppb.New(referenceTime.Add(-3 * day)),
							VariantHash:   pbutil.VariantHash(var3),
							ExpectedCount: 1,
							VerdictCounts: &pb.QueryTestHistoryStatsResponse_Group_VerdictCounts{
								Passed: 1,
							},
						},
					},
				}))
			})
		})

		t.Run("QueryVariants", func(t *ftt.Test) {
			req := &pb.QueryVariantsRequest{
				Project:              "project",
				TestId:               "test_id",
				SubRealm:             "realm",
				FollowTestIdRenaming: true,
				PageSize:             2,
			}

			t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				res, err := server.QueryVariants(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission`))
				assert.Loosely(t, err, should.ErrLike(`in realm "project:realm"`))
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid requests are rejected", func(t *ftt.Test) {
				req.PageSize = -1
				res, err := server.QueryVariants(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("multi-realms", func(t *ftt.Test) {
				req.PageSize = 0
				req.SubRealm = ""
				req.VariantPredicate = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("key2", "val2"),
					},
				}
				res, err := server.QueryVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryVariantsResponse{
					Variants: []*pb.QueryVariantsResponse_VariantInfo{
						{
							VariantHash: pbutil.VariantHash(var3),
							Variant:     var3,
						},
						{
							VariantHash: pbutil.VariantHash(var4),
							Variant:     var4,
						},
					},
				}))
			})

			t.Run("e2e", func(t *ftt.Test) {
				res, err := server.QueryVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryVariantsResponse{
					Variants: []*pb.QueryVariantsResponse_VariantInfo{
						{
							VariantHash: pbutil.VariantHash(var1),
							Variant:     var1,
						},
						{
							VariantHash: pbutil.VariantHash(var3),
							Variant:     var3,
						},
					},
					NextPageToken: res.NextPageToken,
				}))
				assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = server.QueryVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryVariantsResponse{
					Variants: []*pb.QueryVariantsResponse_VariantInfo{
						{
							VariantHash: pbutil.VariantHash(var2),
							Variant:     var2,
						},
					},
				}))
			})
			t.Run("without follow renames", func(t *ftt.Test) {
				req.FollowTestIdRenaming = false
				req.PageSize = 10
				res, err := server.QueryVariants(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				// One less than the baseline test (var3).
				assert.Loosely(t, len(res.Variants), should.Equal(2))
			})
		})

		t.Run("QueryTests", func(t *ftt.Test) {
			searchClient.TestRealms = []testrealms.TestRealm{
				{
					TestID: "test_id",
					Realm:  "project:realm",
				},
				{
					TestID: "test_id1",
					Realm:  "project:realm",
				},
				{
					TestID: "test_id2",
					Realm:  "project:realm",
				},
				{
					TestID: "test_id3",
					Realm:  "project:other-realm",
				},
				{
					TestID: "test_id4",
					Realm:  "project:forbidden-realm",
				},
			}

			req := &pb.QueryTestsRequest{
				Project:         "project",
				TestIdSubstring: "test_id",
				SubRealm:        "realm",
				PageSize:        2,
			}

			t.Run("unauthorised requests are rejected", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				res, err := server.QueryTests(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission`))
				assert.Loosely(t, err, should.ErrLike(`in realm "project:realm"`))
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("invalid requests are rejected", func(t *ftt.Test) {
				req.PageSize = -1
				res, err := server.QueryTests(ctx, req)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, res, should.BeNil)
			})

			t.Run("multi-realms", func(t *ftt.Test) {
				req.PageSize = 0
				req.SubRealm = ""
				res, err := server.QueryTests(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestsResponse{
					TestIds: []string{"test_id", "test_id1", "test_id2", "test_id3"},
				}))
			})

			t.Run("e2e", func(t *ftt.Test) {
				res, err := server.QueryTests(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestsResponse{
					TestIds:       []string{"test_id", "test_id1"},
					NextPageToken: res.NextPageToken,
				}))
				assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

				req.PageToken = res.NextPageToken
				res, err = server.QueryTests(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&pb.QueryTestsResponse{
					TestIds: []string{"test_id2"},
				}))
			})
		})
	})
}

func TestValidateQueryTestHistoryRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("validateQueryTestHistoryRequest", t, func(t *ftt.Test) {
		req := &pb.QueryTestHistoryRequest{
			Project: "project",
			TestId:  "test_id",
			Predicate: &pb.TestVerdictPredicate{
				SubRealm: "realm",
			},
			PageSize: 5,
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateQueryTestHistoryRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no project", func(t *ftt.Test) {
			req.Project = ""
			err := validateQueryTestHistoryRequest(req)
			assert.Loosely(t, err, should.ErrLike("project: unspecified"))
		})

		t.Run("invalid project", func(t *ftt.Test) {
			req.Project = "project:realm"
			err := validateQueryTestHistoryRequest(req)
			assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
		})

		t.Run("no test_id", func(t *ftt.Test) {
			req.TestId = ""
			err := validateQueryTestHistoryRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id: unspecified"))
		})

		t.Run("invalid test_id", func(t *ftt.Test) {
			req.TestId = "\xFF"
			err := validateQueryTestHistoryRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id: not a valid utf8 string"))
		})

		t.Run("no predicate", func(t *ftt.Test) {
			req.Predicate = nil
			err := validateQueryTestHistoryRequest(req)
			assert.Loosely(t, err, should.ErrLike("predicate"))
			assert.Loosely(t, err, should.ErrLike("unspecified"))
		})

		t.Run("no page size", func(t *ftt.Test) {
			req.PageSize = 0
			err := validateQueryTestHistoryRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("negative page size", func(t *ftt.Test) {
			req.PageSize = -1
			err := validateQueryTestHistoryRequest(req)
			assert.Loosely(t, err, should.ErrLike("page_size"))
			assert.Loosely(t, err, should.ErrLike("negative"))
		})
	})
}

func TestValidateQueryTestHistoryStatsRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("validateQueryTestHistoryStatsRequest", t, func(t *ftt.Test) {
		req := &pb.QueryTestHistoryStatsRequest{
			Project: "project",
			TestId:  "test_id",
			Predicate: &pb.TestVerdictPredicate{
				SubRealm: "realm",
			},
			PageSize: 5,
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateQueryTestHistoryStatsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no project", func(t *ftt.Test) {
			req.Project = ""
			err := validateQueryTestHistoryStatsRequest(req)
			assert.Loosely(t, err, should.ErrLike("project: unspecified"))
		})

		t.Run("invalid project", func(t *ftt.Test) {
			req.Project = "project:realm"
			err := validateQueryTestHistoryStatsRequest(req)
			assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
		})

		t.Run("no test_id", func(t *ftt.Test) {
			req.TestId = ""
			err := validateQueryTestHistoryStatsRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id: unspecified"))
		})

		t.Run("invalid test_id", func(t *ftt.Test) {
			req.TestId = "\xFF"
			err := validateQueryTestHistoryStatsRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id: not a valid utf8 string"))
		})

		t.Run("no predicate", func(t *ftt.Test) {
			req.Predicate = nil
			err := validateQueryTestHistoryStatsRequest(req)
			assert.Loosely(t, err, should.ErrLike("predicate"))
			assert.Loosely(t, err, should.ErrLike("unspecified"))
		})

		t.Run("no page size", func(t *ftt.Test) {
			req.PageSize = 0
			err := validateQueryTestHistoryStatsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("negative page size", func(t *ftt.Test) {
			req.PageSize = -1
			err := validateQueryTestHistoryStatsRequest(req)
			assert.Loosely(t, err, should.ErrLike("page_size"))
			assert.Loosely(t, err, should.ErrLike("negative"))
		})
	})
}

func TestValidateQueryVariantsRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("validateQueryVariantsRequest", t, func(t *ftt.Test) {
		req := &pb.QueryVariantsRequest{
			Project:  "project",
			TestId:   "test_id",
			PageSize: 5,
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateQueryVariantsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no project", func(t *ftt.Test) {
			req.Project = ""
			err := validateQueryVariantsRequest(req)
			assert.Loosely(t, err, should.ErrLike("project: unspecified"))
		})

		t.Run("invalid project", func(t *ftt.Test) {
			req.Project = "project:realm"
			err := validateQueryVariantsRequest(req)
			assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
		})

		t.Run("no test_id", func(t *ftt.Test) {
			req.TestId = ""
			err := validateQueryVariantsRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id: unspecified"))
		})

		t.Run("invalid test_id", func(t *ftt.Test) {
			req.TestId = "\xFF"
			err := validateQueryVariantsRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id: not a valid utf8 string"))
		})

		t.Run("bad sub_realm", func(t *ftt.Test) {
			req.SubRealm = "a:realm"
			err := validateQueryVariantsRequest(req)
			assert.Loosely(t, err, should.ErrLike("sub_realm: bad project-scoped realm name"))
		})

		t.Run("no page size", func(t *ftt.Test) {
			req.PageSize = 0
			err := validateQueryVariantsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("negative page size", func(t *ftt.Test) {
			req.PageSize = -1
			err := validateQueryVariantsRequest(req)
			assert.Loosely(t, err, should.ErrLike("page_size"))
			assert.Loosely(t, err, should.ErrLike("negative"))
		})
	})
}

func TestValidateQueryTestsRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("validateQueryTestsRequest", t, func(t *ftt.Test) {
		req := &pb.QueryTestsRequest{
			Project:         "project",
			TestIdSubstring: "test_id",
			PageSize:        5,
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateQueryTestsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no project", func(t *ftt.Test) {
			req.Project = ""
			err := validateQueryTestsRequest(req)
			assert.Loosely(t, err, should.ErrLike("project: unspecified"))
		})

		t.Run("invalid project", func(t *ftt.Test) {
			req.Project = "project:realm"
			err := validateQueryTestsRequest(req)
			assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
		})

		t.Run("no test_id_substring", func(t *ftt.Test) {
			req.TestIdSubstring = ""
			err := validateQueryTestsRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id_substring: unspecified"))
		})

		t.Run("bad test_id_substring", func(t *ftt.Test) {
			req.TestIdSubstring = "\xFF"
			err := validateQueryTestsRequest(req)
			assert.Loosely(t, err, should.ErrLike("test_id_substring: not a valid utf8 string"))
		})

		t.Run("bad sub_realm", func(t *ftt.Test) {
			req.SubRealm = "a:realm"
			err := validateQueryTestsRequest(req)
			assert.Loosely(t, err, should.ErrLike("sub_realm: bad project-scoped realm name"))
		})

		t.Run("no page size", func(t *ftt.Test) {
			req.PageSize = 0
			err := validateQueryTestsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("negative page size", func(t *ftt.Test) {
			req.PageSize = -1
			err := validateQueryTestsRequest(req)
			assert.Loosely(t, err, should.ErrLike("page_size"))
			assert.Loosely(t, err, should.ErrLike("negative"))
		})
	})
}
