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
	"encoding/hex"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/testrealms"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestQuerySourceVerdicts(t *testing.T) {
	ftt.Run("QuerySourceVerdicts", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "test-project:testrealm", Permission: rdbperms.PermListTestResults},
				{Realm: "test-project:otherrealm", Permission: rdbperms.PermListTestResults},
			},
		})

		testData := lowlatency.CreateTestData()
		testutil.MustApply(ctx, t, testData...)

		expectedVerdicts := lowlatency.ExpectedSourceVerdictsProto()

		searchClient := &testrealms.FakeClient{}
		server := NewTestHistoryServer(searchClient)

		req := &pb.QuerySourceVerdictsV2Request{
			Project: "test-project",
			Test: &pb.QuerySourceVerdictsV2Request_TestIdFlat{
				TestIdFlat: &pb.FlatTestIdentifier{
					TestId:      "test-id",
					VariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
				},
			},
			SourceRef: lowlatency.TestRef,
		}

		t.Run("Request validation", func(t *ftt.Test) {
			t.Run("Project", func(t *ftt.Test) {
				t.Run("Unspecified", func(t *ftt.Test) {
					req.Project = ""
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("project: unspecified"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
			})

			t.Run("Test", func(t *ftt.Test) {
				t.Run("Unspecified", func(t *ftt.Test) {
					req.Test = nil
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("either test_id_structured or test_id_flat must be specified"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
				t.Run("Invalid flat test ID", func(t *ftt.Test) {
					req.Test = &pb.QuerySourceVerdictsV2Request_TestIdFlat{
						TestIdFlat: &pb.FlatTestIdentifier{
							TestId:      "",
							VariantHash: "e5aa3a34a834a74f",
						},
					}
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("test_id_flat: test_id: unspecified"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
				t.Run("Invalid structured test ID", func(t *ftt.Test) {
					req.Test = &pb.QuerySourceVerdictsV2Request_TestIdStructured{
						TestIdStructured: &pb.TestIdentifier{
							ModuleName: "module",
						},
					}
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("test_id_structured: module_scheme: unspecified"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
			})

			t.Run("Source ref", func(t *ftt.Test) {
				t.Run("Unspecified", func(t *ftt.Test) {
					req.SourceRef = nil
					req.SourceRefHash = ""
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("either source_ref or source_ref_hash must be specified"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
				t.Run("Invalid source ref", func(t *ftt.Test) {
					req.SourceRef = &pb.SourceRef{
						System: &pb.SourceRef_Gitiles{
							Gitiles: &pb.GitilesRef{
								Host: "chromium/src",
								Ref:  "refs/heads/main",
							},
						},
					}
					req.SourceRefHash = ""
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("source_ref: gitiles: project: unspecified"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
				t.Run("Invalid source ref hash", func(t *ftt.Test) {
					req.SourceRef = nil
					req.SourceRefHash = "a"
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("source_ref_hash: source ref hash a must match ^[0-9a-f]{16}$"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
				t.Run("Both specified", func(t *ftt.Test) {
					req.SourceRef = lowlatency.TestRef
					req.SourceRefHash = strings.Repeat("a", 16)
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("only one of source_ref and source_ref_hash can be specified"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
			})

			t.Run("Page size", func(t *ftt.Test) {
				t.Run("Invalid", func(t *ftt.Test) {
					req.PageSize = -1
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("page_size: must be non-negative"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
			})

			t.Run("Filter", func(t *ftt.Test) {
				t.Run("Invalid", func(t *ftt.Test) {
					req := proto.Clone(req).(*pb.QuerySourceVerdictsV2Request)
					req.Filter = "invalid = filter"
					_, err := server.QuerySourceVerdicts(ctx, req)
					assert.Loosely(t, err, should.ErrLike("filter: no filterable field \"invalid\" or a prefix thereof"))
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				})
			})
		})

		t.Run("Unauthorised requests are rejected", func(t *ftt.Test) {
			// No permission at all.
			unauthCtx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			_, err := server.QuerySourceVerdicts(unauthCtx, req)
			assert.Loosely(t, err, should.ErrLike(`caller does not have permissions [resultdb.testResults.list] in any realm in project "test-project"`))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("Baseline", func(t *ftt.Test) {
			res, err := server.QuerySourceVerdicts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.SourceVerdicts, should.Match(expectedVerdicts))
		})
		t.Run("With source ref hash instead of source ref", func(t *ftt.Test) {
			req.SourceRef = nil
			req.SourceRefHash = hex.EncodeToString(pbutil.SourceRefHash(lowlatency.TestRef))

			res, err := server.QuerySourceVerdicts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.SourceVerdicts, should.Match(expectedVerdicts))
		})
		t.Run("With structured test ID instead of flat test ID", func(t *ftt.Test) {
			req.Test = &pb.QuerySourceVerdictsV2Request_TestIdStructured{
				TestIdStructured: &pb.TestIdentifier{
					ModuleName:    "legacy",
					ModuleScheme:  "legacy",
					ModuleVariant: pbutil.Variant("key", "value"),
					CaseName:      "test-id",
				},
			}

			res, err := server.QuerySourceVerdicts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.SourceVerdicts, should.Match(expectedVerdicts))
		})
		t.Run("With filter", func(t *ftt.Test) {
			req.Filter = "position = 98"

			expectedVerdicts := lowlatency.FilterSourceVerdictsProtos(expectedVerdicts, func(v *pb.SourceVerdict) bool {
				return v.Position == 98
			})
			assert.Loosely(t, expectedVerdicts, should.HaveLength(1))

			res, err := server.QuerySourceVerdicts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.SourceVerdicts, should.Match(expectedVerdicts))
		})

		t.Run("With pagination", func(t *ftt.Test) {
			req.PageSize = 4

			// Page 1
			res, err := server.QuerySourceVerdicts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.SourceVerdicts, should.Match(expectedVerdicts[0:4]))
			assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

			// Page 2
			req.PageToken = res.NextPageToken
			res, err = server.QuerySourceVerdicts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.SourceVerdicts, should.Match(expectedVerdicts[4:8]))
			assert.Loosely(t, res.NextPageToken, should.NotBeEmpty)

			// Page 3
			req.PageToken = res.NextPageToken
			res, err = server.QuerySourceVerdicts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.SourceVerdicts, should.Match(expectedVerdicts[8:]))
			assert.Loosely(t, res.NextPageToken, should.BeEmpty)
		})

		t.Run("With limited access", func(t *ftt.Test) {
			// Change permissions to only access the otherrealm
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "test-project:otherrealm", Permission: rdbperms.PermListTestResults},
				},
			})

			expectedVerdicts := lowlatency.FilterSourceVerdictsProtos(expectedVerdicts, func(v *pb.SourceVerdict) bool {
				// This is the only verdict in realm otherrealm.
				return v.Position == 91
			})
			assert.Loosely(t, expectedVerdicts, should.HaveLength(1))

			res, err := server.QuerySourceVerdicts(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.SourceVerdicts, should.Match(expectedVerdicts))
		})
	})
}
