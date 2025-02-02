// Copyright 2024 The LUCI Authors.
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
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/tree_status/internal/config"
	"go.chromium.org/luci/tree_status/internal/perms"
	"go.chromium.org/luci/tree_status/internal/testutil"
	pb "go.chromium.org/luci/tree_status/proto/v1"
)

func TestTrees(t *testing.T) {
	ftt.Run("With a trees server", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)

		// For config.
		ctx = memory.Use(ctx)
		testConfig := config.TestConfig()
		err := config.SetConfig(ctx, testConfig)
		assert.Loosely(t, err, should.BeNil)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-tree-status-access", "googlers"},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewTreesServer()

		t.Run("GetTree", func(t *ftt.Test) {
			t.Run("Empty tree name", func(t *ftt.Test) {
				request := &pb.GetTreeRequest{}
				_, err := server.GetTree(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("name: must be specified"))
			})
			t.Run("Invalid tree name", func(t *ftt.Test) {
				request := &pb.GetTreeRequest{
					Name: "invalid",
				}
				_, err := server.GetTree(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("name: expected format"))
			})
			t.Run("Tree not found", func(t *ftt.Test) {
				request := &pb.GetTreeRequest{
					Name: "trees/random-tree",
				}
				_, err := server.GetTree(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`tree "random-tree" not found`))
			})
			t.Run("Default ACLs anonymous", func(t *ftt.Test) {
				ctx = perms.FakeAuth().Anonymous().SetInContext(ctx)
				request := &pb.GetTreeRequest{
					Name: "trees/chromium",
				}
				_, err := server.GetTree(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("please log in for access"))
			})
			t.Run("Default ACLs no read access", func(t *ftt.Test) {
				ctx = perms.FakeAuth().SetInContext(ctx)
				request := &pb.GetTreeRequest{
					Name: "trees/chromium",
				}
				_, err := server.GetTree(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`user is not a member of group "luci-tree-status-access"`))
			})
			t.Run("Realm-based ACLs no access", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = perms.FakeAuth().SetInContext(ctx)
				request := &pb.GetTreeRequest{
					Name: "trees/chromium",
				}
				_, err = server.GetTree(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user does not have permission to perform this action"))
			})
			t.Run("Default ACLs get tree successfully", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().SetInContext(ctx)
				request := &pb.GetTreeRequest{
					Name: "trees/chromium",
				}
				tree, err := server.GetTree(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, tree, should.Match(&pb.Tree{
					Name:     "trees/chromium",
					Projects: []string{"chromium"},
				}))
			})
			t.Run("Realm-based ACLs get tree successfully", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermGetTree, "chromium:@project").SetInContext(ctx)
				request := &pb.GetTreeRequest{
					Name: "trees/chromium",
				}
				tree, err := server.GetTree(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, tree, should.Match(&pb.Tree{
					Name:     "trees/chromium",
					Projects: []string{"chromium"},
				}))
			})
		})

		t.Run("QueryTrees", func(t *ftt.Test) {
			t.Run("No project returns empty response", func(t *ftt.Test) {
				request := &pb.QueryTreesRequest{
					Project: "nothing",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{}))
			})
			t.Run("Default ACLs anonymous returns empty response", func(t *ftt.Test) {
				ctx = perms.FakeAuth().Anonymous().SetInContext(ctx)

				request := &pb.QueryTreesRequest{
					Project: "chromium",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{}))
			})
			t.Run("Default ACLs no read access returns empty response", func(t *ftt.Test) {
				ctx = perms.FakeAuth().SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "chromium",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{}))
			})
			t.Run("Realm-based ACLs no read access returns empty response", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "chromium",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{}))
			})

			t.Run("Default ACLs successful query", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "chromium",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{
					Trees: []*pb.Tree{
						{
							Name:     "trees/chromium",
							Projects: []string{"chromium"},
						},
					},
				}))
			})

			t.Run("Default ACLs successful query more than 1 trees", func(t *ftt.Test) {
				testConfig.Trees[1].Name = "chromium1"
				testConfig.Trees[1].Projects = []string{"chromium"}
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithReadAccess().SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "chromium",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{
					Trees: []*pb.Tree{
						{
							Name:     "trees/chromium",
							Projects: []string{"chromium"},
						},
						{
							Name:     "trees/chromium1",
							Projects: []string{"chromium"},
						},
					},
				}))
			})

			t.Run("Realm-based successful query", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermListTree, "chromium:@project").SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "chromium",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{
					Trees: []*pb.Tree{
						{
							Name:     "trees/chromium",
							Projects: []string{"chromium"},
						},
					},
				}))
			})

			t.Run("Realm-based successful query more than 1 tree", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermListTree, "pigweed:subrealm").WithPermissionInRealm(perms.PermListTree, "pigweed:subrealm2").SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "pigweed",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{
					Trees: []*pb.Tree{
						{
							Name:     "trees/pigweed",
							Projects: []string{"pigweed"},
						},
						{
							Name:     "trees/pigweed2",
							Projects: []string{"pigweed"},
						},
					},
				}))
			})

			t.Run("Realm-based successful has 2 trees but only 1 returned", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermListTree, "pigweed:subrealm2").SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "pigweed",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{
					Trees: []*pb.Tree{
						{
							Name:     "trees/pigweed2",
							Projects: []string{"pigweed"},
						},
					},
				}))
			})

			t.Run("Mixed ACLs", func(t *ftt.Test) {
				testConfig.Trees[4].UseDefaultAcls = true
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermListTree, "pigweed:subrealm").WithReadAccess().SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "pigweed",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{
					Trees: []*pb.Tree{
						{
							Name:     "trees/pigweed",
							Projects: []string{"pigweed"},
						},
						{
							Name:     "trees/pigweed2",
							Projects: []string{"pigweed"},
						},
					},
				}))
			})
		})
	})
}
