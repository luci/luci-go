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
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestGetRootInvocation(t *testing.T) {
	ftt.Run("GetRootInvocation", t, func(t *ftt.Test) {
		const realm = "testproject:testrealm"

		ctx := testutil.SpannerTestContext(t)

		// Insert a root invocation.
		testData := rootinvocations.NewBuilder("root-inv-id").
			WithFinalizationState(pb.RootInvocation_FINALIZED).
			WithRealm(realm).Build()
		testutil.MustApply(ctx, t, insert.RootInvocationOnly(testData)...)

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

		t.Run("happy path", func(t *ftt.Test) {
			expectedRootInvocation := &pb.RootInvocation{
				Name:                 "rootInvocations/root-inv-id",
				RootInvocationId:     "root-inv-id",
				Realm:                realm,
				FinalizationState:    pb.RootInvocation_FINALIZED,
				State:                testData.State,
				SummaryMarkdown:      testData.SummaryMarkdown,
				CreateTime:           pbutil.MustTimestampProto(testData.CreateTime),
				Creator:              testData.CreatedBy,
				LastUpdated:          pbutil.MustTimestampProto(testData.LastUpdated),
				FinalizeTime:         pbutil.MustTimestampProto(testData.FinalizeTime.Time),
				FinalizeStartTime:    pbutil.MustTimestampProto(testData.FinalizeStartTime.Time),
				Sources:              testData.Sources,
				StreamingExportState: testData.StreamingExportState,
				Tags:                 testData.Tags,
				Properties:           testData.Properties,
				BaselineId:           testData.BaselineID,
				Etag:                 `W/"2025-04-26T01:02:03.000004Z"`,
			}

			rsp, err := srv.GetRootInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rsp, should.Match(expectedRootInvocation))
		})

		t.Run("does not exist", func(t *ftt.Test) {
			req.Name = "rootInvocations/non-existent"
			_, err := srv.GetRootInvocation(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`"rootInvocations/non-existent" not found`))
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
