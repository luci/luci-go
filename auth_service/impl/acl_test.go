// Copyright 2021 The LUCI Authors.
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

package impl

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/auth_service/impl/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAuthorizeRPCAccess(t *testing.T) {
	t.Parallel()

	interceptor := AuthorizeRPCAccess.Unary()

	check := func(ctx context.Context, service, method string) codes.Code {
		info := &grpc.UnaryServerInfo{
			FullMethod: fmt.Sprintf("/%s/%s", service, method),
		}
		_, err := interceptor(ctx, nil, info, func(context.Context, any) (any, error) {
			return nil, nil
		})
		return status.Code(err)
	}

	Convey("Anonymous", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{})

		So(check(ctx, "auth.service.Accounts", "GetSelf"), ShouldEqual, codes.OK)
		So(check(ctx, "discovery.Discovery", "Something"), ShouldEqual, codes.OK)
		So(check(ctx, "auth.service.Groups", "Something"), ShouldEqual, codes.PermissionDenied)
		So(check(ctx, "auth.service.Groups", "CreateGroup"), ShouldEqual, codes.PermissionDenied)
		So(check(ctx, "unknown.API", "Something"), ShouldEqual, codes.PermissionDenied)
	})

	Convey("Authenticated, but not authorized", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"some-random-group"},
		})

		So(check(ctx, "auth.service.Accounts", "GetSelf"), ShouldEqual, codes.OK)
		So(check(ctx, "discovery.Discovery", "Something"), ShouldEqual, codes.OK)
		So(check(ctx, "auth.service.Groups", "Something"), ShouldEqual, codes.PermissionDenied)
		So(check(ctx, "auth.service.Groups", "CreateGroup"), ShouldEqual, codes.PermissionDenied)
		So(check(ctx, "unknown.API", "Something"), ShouldEqual, codes.PermissionDenied)
	})

	Convey("Authorized", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{authdb.AuthServiceAccessGroup},
		})

		So(check(ctx, "auth.service.Accounts", "GetSelf"), ShouldEqual, codes.OK)
		So(check(ctx, "discovery.Discovery", "Something"), ShouldEqual, codes.OK)
		So(check(ctx, "auth.service.Groups", "Something"), ShouldEqual, codes.OK)
		So(check(ctx, "auth.service.Groups", "CreateGroup"), ShouldEqual, codes.PermissionDenied)
		So(check(ctx, "unknown.API", "Something"), ShouldEqual, codes.PermissionDenied)
	})

	Convey("Authorized as admin", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{authdb.AuthServiceAccessGroup, model.AdminGroup},
		})

		So(check(ctx, "auth.service.Accounts", "GetSelf"), ShouldEqual, codes.OK)
		So(check(ctx, "discovery.Discovery", "Something"), ShouldEqual, codes.OK)
		So(check(ctx, "auth.service.Groups", "Something"), ShouldEqual, codes.OK)
		So(check(ctx, "auth.service.Groups", "CreateGroup"), ShouldEqual, codes.OK)
		So(check(ctx, "unknown.API", "Something"), ShouldEqual, codes.PermissionDenied)
	})
}
