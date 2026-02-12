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

	"go.chromium.org/luci/analysis/internal/testrealms"
	"go.chromium.org/luci/analysis/internal/testutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestQueryTests(t *testing.T) {
	ftt.Run("QueryTests", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:            "user:someone@example.com",
			IdentityPermissions: defaultAuthPermissions,
		})
		now := referenceTime.Add(time.Minute * 20)
		ctx, _ = testclock.UseTime(ctx, now)

		searchClient := &testrealms.FakeClient{
			TestRealms: []testrealms.TestRealm{
				{
					TestID:   "test_id",
					TestName: "test_name",
					Realm:    "project:realm",
				},
				{
					TestID:   "test_id1",
					TestName: "a_special_name",
					Realm:    "project:realm",
				},
				{
					TestID:   "test_id2",
					TestName: "a_special_name2",
					Realm:    "project:realm",
				},
				{
					TestID:   "test_id3",
					TestName: "a_special_name3",
					Realm:    "project:other-realm",
				},
				{
					TestID:   "test_id4",
					TestName: "a_special_name4",
					Realm:    "project:forbidden-realm",
				},
			},
		}
		server := NewTestHistoryServer(searchClient)

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

		t.Run("search on test name", func(t *ftt.Test) {
			req.PageSize = 0
			req.TestIdSubstring = "special_name"
			res, err := server.QueryTests(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(&pb.QueryTestsResponse{
				TestIds: []string{"test_id1", "test_id2"},
			}))
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
