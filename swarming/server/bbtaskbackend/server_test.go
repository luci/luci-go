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

package bbtaskbackend

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskBackendRPCAccess(t *testing.T) {
	t.Parallel()

	Convey("TaskBackendAuthInterceptor", t, func() {
		interceptor := TaskBackendAuthInterceptor(true)
		ctx := context.Background()
		info := &grpc.UnaryServerInfo{
			FullMethod: "/buildbucket.v2.TaskBackend/FetchTasks",
		}

		Convey("auth state is not set", func() {
			_, err := interceptor(ctx, nil, info, func(context.Context, any) (any, error) {
				return nil, nil
			})
			So(err, ShouldHaveGRPCStatus, codes.Internal, "the auth state is not properly configured")
		})

		Convey("not from Buildbucket", func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				PeerIdentityOverride: identity.Identity("user:foo@appspot.gserviceaccount.com"),
			})

			_, err := interceptor(ctx, nil, info, func(context.Context, any) (any, error) {
				return nil, nil
			})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied, `peer "user:foo@appspot.gserviceaccount.com" is not allowed to access this task backend`)
		})

		Convey("not a project identity", func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:             "user:someone@example.com",
				PeerIdentityOverride: identity.Identity("user:cr-buildbucket-dev@appspot.gserviceaccount.com"),
			})

			_, err := interceptor(ctx, nil, info, func(context.Context, any) (any, error) {
				return nil, nil
			})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied, `caller's user identity "user:someone@example.com" is not a project identity`)
		})

		Convey("ok", func() {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:             "project:foo",
				PeerIdentityOverride: identity.Identity("user:cr-buildbucket-dev@appspot.gserviceaccount.com"),
			})

			_, err := interceptor(ctx, nil, info, func(context.Context, any) (any, error) {
				return nil, nil
			})
			So(err, ShouldBeNil)
		})

		Convey("not calling TaskBackend service", func() {
			info := &grpc.UnaryServerInfo{
				FullMethod: "/another.v1.service/abc",
			}

			_, err := interceptor(ctx, nil, info, func(context.Context, any) (any, error) {
				return nil, nil
			})
			So(err, ShouldBeNil)
		})
	})
}
