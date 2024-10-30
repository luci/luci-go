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

package resultdb

import (
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateQueryRootInvocationNamesRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`validateQueryRootInvocationNamesRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			req := &pb.QueryRootInvocationNamesRequest{Name: "invocations/valid_id_0"}
			assert.Loosely(t, validateQueryRootInvocationNamesRequest(req), should.BeNil)
		})

		t.Run(`Invalid name`, func(t *ftt.Test) {
			t.Run(`, missing`, func(t *ftt.Test) {
				req := &pb.QueryRootInvocationNamesRequest{}
				assert.Loosely(t, validateQueryRootInvocationNamesRequest(req), should.ErrLike("name missing"))
			})

			t.Run(`, invalid format`, func(t *ftt.Test) {
				req := &pb.QueryRootInvocationNamesRequest{Name: "bad_name"}
				assert.Loosely(t, validateQueryRootInvocationNamesRequest(req), should.ErrLike("does not match"))
			})
		})
	})
}

func TestQueryRootInvocationNames(t *testing.T) {
	ftt.Run(`QueryRootInvocationNames`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetInvocation},
			},
		})

		srv := newTestResultDBService()

		t.Run(`Valid`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil, "d"),
				[]*spanner.Mutation{insert.Invocation("d", pb.Invocation_ACTIVE, nil)},
			)...)

			inv, err := srv.QueryRootInvocationNames(ctx, &pb.QueryRootInvocationNamesRequest{Name: "invocations/d"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Resemble(&pb.QueryRootInvocationNamesResponse{
				RootInvocationNames: []string{"invocations/a"},
			}))
		})

		t.Run(`Permission denied`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("secret", pb.Invocation_ACTIVE, map[string]any{
					"Realm": "secretproject:testrealm",
				}),
			)
			req := &pb.QueryRootInvocationNamesRequest{Name: "invocations/secret"}
			_, err := srv.QueryRootInvocationNames(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.invocations.get"))
		})
	})
}
