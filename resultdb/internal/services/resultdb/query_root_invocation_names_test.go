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

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryRootInvocationNamesRequest(t *testing.T) {
	t.Parallel()
	Convey(`validateQueryRootInvocationNamesRequest`, t, func() {
		Convey(`Valid`, func() {
			req := &pb.QueryRootInvocationNamesRequest{Name: "invocations/valid_id_0"}
			So(validateQueryRootInvocationNamesRequest(req), ShouldBeNil)
		})

		Convey(`Invalid name`, func() {
			Convey(`, missing`, func() {
				req := &pb.QueryRootInvocationNamesRequest{}
				So(validateQueryRootInvocationNamesRequest(req), ShouldErrLike, "name missing")
			})

			Convey(`, invalid format`, func() {
				req := &pb.QueryRootInvocationNamesRequest{Name: "bad_name"}
				So(validateQueryRootInvocationNamesRequest(req), ShouldErrLike, "does not match")
			})
		})
	})
}

func TestQueryRootInvocationNames(t *testing.T) {
	Convey(`QueryRootInvocationNames`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetInvocation},
			},
		})

		srv := newTestResultDBService()

		Convey(`Valid`, func() {
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.InvocationWithInclusions("a", pb.Invocation_ACTIVE, nil, "b"),
				insert.InvocationWithInclusions("b", pb.Invocation_ACTIVE, nil, "c"),
				insert.InvocationWithInclusions("c", pb.Invocation_ACTIVE, nil, "d"),
				[]*spanner.Mutation{insert.Invocation("d", pb.Invocation_ACTIVE, nil)},
			)...)

			inv, err := srv.QueryRootInvocationNames(ctx, &pb.QueryRootInvocationNamesRequest{Name: "invocations/d"})
			So(err, ShouldBeNil)
			So(inv, ShouldResembleProto, &pb.QueryRootInvocationNamesResponse{
				RootInvocationNames: []string{"invocations/a"},
			})
		})

		Convey(`Permission denied`, func() {
			testutil.MustApply(ctx,
				insert.Invocation("secret", pb.Invocation_ACTIVE, map[string]any{
					"Realm": "secretproject:testrealm",
				}),
			)
			req := &pb.QueryRootInvocationNamesRequest{Name: "invocations/secret"}
			_, err := srv.QueryRootInvocationNames(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.invocations.get")
		})
	})
}
