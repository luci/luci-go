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
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/testrealms"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestQueryVariants(t *testing.T) {
	ftt.Run("QueryVariants", t, func(t *ftt.Test) {
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

		t.Run("test metadata permission denied errors are forwarded to client", func(t *ftt.Test) {
			fakeRDBClient.IsAccessDenied = true
			_, err := server.QueryVariants(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission to query for previous test ID"))
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
