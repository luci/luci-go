// Copyright 2026 The LUCI Authors.
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

package testhistory

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/testrealms"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestQueryTestHistoryStats(t *testing.T) {
	ftt.Run("QueryStats", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:            "user:someone@example.com",
			IdentityPermissions: defaultAuthPermissions,
		})
		now := referenceTime.Add(time.Minute * 20)
		ctx, _ = testclock.UseTime(ctx, now)

		err := createTestResultData(ctx, t)
		assert.Loosely(t, err, should.BeNil)

		// Install a fake ResultDB test metadata client in the context.
		// This supports following test renaming.
		fakeRDBClient := createFakeTestMetadataClient()
		ctx = resultdb.UseClientForTesting(ctx, fakeRDBClient)

		searchClient := &testrealms.FakeClient{}
		server := NewTestHistoryServer(searchClient)

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

		t.Run("test metadata permission denied errors are forwarded to client", func(t *ftt.Test) {
			fakeRDBClient.IsAccessDenied = true
			_, err := server.QueryStats(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission to query for previous test ID"))
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
