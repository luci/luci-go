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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/tree_status/internal/config"
	"go.chromium.org/luci/tree_status/internal/perms"
	"go.chromium.org/luci/tree_status/internal/testutil"
	pb "go.chromium.org/luci/tree_status/proto/v1"

	. "go.chromium.org/luci/common/testing/assertions"
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

		t.Run("QueryTrees", func(t *ftt.Test) {
			t.Run("No project returns empty tree", func(t *ftt.Test) {
				request := &pb.QueryTreesRequest{
					Project: "nothing",
				}
				res, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(&pb.QueryTreesResponse{}))
			})
			t.Run("Default ACLs anonymous rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().Anonymous().SetInContext(ctx)

				request := &pb.QueryTreesRequest{
					Project: "chromium",
				}
				_, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("log in"))
			})
			t.Run("Default ACLs no read access rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "chromium",
				}
				_, err := server.QueryTrees(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("user is not a member of group \"luci-tree-status-access\""))
			})
			t.Run("Realm-based ACLs no read access rejected", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().SetInContext(ctx)
				request := &pb.QueryTreesRequest{
					Project: "chromium",
				}
				_, err = server.QueryTrees(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("user does not have permission to perform this action"))
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
							Name: "trees/chromium",
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
							Name: "trees/chromium",
						},
					},
				}))
			})
		})
	})
}
