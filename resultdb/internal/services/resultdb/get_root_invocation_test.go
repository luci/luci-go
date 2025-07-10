// Copyright 2025 The LUCI Authors.
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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestGetRootInvocation(t *testing.T) {
	ftt.Run("GetRootInvocation", t, func(t *ftt.Test) {
		const realm = "testproject:testrealm"

		ctx := testutil.SpannerTestContext(t)

		// Insert a root invocation.
		testData := rootinvocations.NewBuilder("root-inv-id").
			WithState(pb.RootInvocation_FINALIZED).
			WithRealm(realm).Build()
		testutil.MustApply(ctx, t, insert.RootInvocation(testData)...)

		// Setup authorisation.
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: realm, Permission: rdbperms.PermGetRootInvocation},
			},
		}
		ctx = auth.WithState(ctx, authState)

		req := &pb.GetRootInvocationRequest{
			Name: "rootInvocations/root-inv-id",
		}

		srv := newTestResultDBService()

		t.Run("valid", func(t *ftt.Test) {
			rsp, err := srv.GetRootInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rsp.Name, should.Equal("rootInvocations/root-inv-id"))
			assert.That(t, rsp.RootInvocationId, should.Equal("root-inv-id"))
			assert.That(t, rsp.State, should.Equal(pb.RootInvocation_FINALIZED))
			assert.That(t, rsp.Realm, should.Equal(realm))
			assert.That(t, rsp.CreateTime.AsTime(), should.Match(testData.CreateTime))
			assert.That(t, rsp.Creator, should.Equal(testData.CreatedBy))
			assert.That(t, rsp.FinalizeStartTime.AsTime(), should.Match(testData.FinalizeStartTime.Time))
			assert.That(t, rsp.FinalizeTime.AsTime(), should.Match(testData.FinalizeTime.Time))
			assert.That(t, rsp.Deadline.AsTime(), should.Match(testData.Deadline))
			assert.That(t, rsp.ProducerResource, should.Equal(testData.ProducerResource))
			assert.That(t, rsp.Sources, should.Match(testData.Sources))
			assert.That(t, rsp.SourcesFinal, should.BeTrue)
			assert.That(t, rsp.Tags, should.Match(testData.Tags))
			assert.That(t, rsp.Properties, should.Match(testData.Properties))
			assert.That(t, rsp.BaselineId, should.Equal("baseline"))
		})

		t.Run("does not exist", func(t *ftt.Test) {
			req.Name = "rootInvocations/non-existent"
			_, err := srv.GetRootInvocation(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike("rootInvocations/non-existent not found"))
		})

		t.Run("permission denied", func(t *ftt.Test) {
			authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermGetRootInvocation)

			_, err := srv.GetRootInvocation(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("caller does not have permission resultdb.rootInvocations.get"))
		})

		t.Run("invalid request", func(t *ftt.Test) {
			t.Run("name", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Name = "invalid-name"
					_, err := srv.GetRootInvocation(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("name: does not match"))
				})
				t.Run("empty", func(t *ftt.Test) {
					req.Name = ""
					_, err := srv.GetRootInvocation(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("name: unspecified"))
				})
			})
		})
	})
}
