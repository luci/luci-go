// Copyright 2022 The LUCI Authors.
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

package rpcacl

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInterceptor(t *testing.T) {
	t.Parallel()

	interceptor := Interceptor(Map{
		"/all/*":               All,
		"/authenticated/*":     Authenticated,
		"/authorized/*":        "some-group",
		"/mixed/all":           All,
		"/mixed/authenticated": Authenticated,
		"/mixed/authorized":    "some-group",
	}).Unary()

	check := func(ctx context.Context, service, method string) codes.Code {
		info := &grpc.UnaryServerInfo{
			FullMethod: fmt.Sprintf("/%s/%s", service, method),
		}
		_, err := interceptor(ctx, nil, info, func(context.Context, interface{}) (interface{}, error) {
			return nil, nil
		})
		return status.Code(err)
	}

	Convey("Anonymous", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{})

		So(check(ctx, "all", "method"), ShouldEqual, codes.OK)
		So(check(ctx, "authenticated", "method"), ShouldEqual, codes.Unauthenticated)
		So(check(ctx, "authorized", "method"), ShouldEqual, codes.PermissionDenied)
		So(check(ctx, "unknown", "method"), ShouldEqual, codes.PermissionDenied)

		So(check(ctx, "mixed", "all"), ShouldEqual, codes.OK)
		So(check(ctx, "mixed", "authenticated"), ShouldEqual, codes.Unauthenticated)
		So(check(ctx, "mixed", "authorized"), ShouldEqual, codes.PermissionDenied)
		So(check(ctx, "mixed", "unknown"), ShouldEqual, codes.PermissionDenied)
	})

	Convey("Authenticated, but not authorized", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"some-random-group"},
		})

		So(check(ctx, "all", "method"), ShouldEqual, codes.OK)
		So(check(ctx, "authenticated", "method"), ShouldEqual, codes.OK)
		So(check(ctx, "authorized", "method"), ShouldEqual, codes.PermissionDenied)
		So(check(ctx, "unknown", "method"), ShouldEqual, codes.PermissionDenied)

		So(check(ctx, "mixed", "all"), ShouldEqual, codes.OK)
		So(check(ctx, "mixed", "authenticated"), ShouldEqual, codes.OK)
		So(check(ctx, "mixed", "authorized"), ShouldEqual, codes.PermissionDenied)
		So(check(ctx, "mixed", "unknown"), ShouldEqual, codes.PermissionDenied)
	})

	Convey("Authorized", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"some-group"},
		})

		So(check(ctx, "all", "method"), ShouldEqual, codes.OK)
		So(check(ctx, "authenticated", "method"), ShouldEqual, codes.OK)
		So(check(ctx, "authorized", "method"), ShouldEqual, codes.OK)
		So(check(ctx, "unknown", "method"), ShouldEqual, codes.PermissionDenied)

		So(check(ctx, "mixed", "all"), ShouldEqual, codes.OK)
		So(check(ctx, "mixed", "authenticated"), ShouldEqual, codes.OK)
		So(check(ctx, "mixed", "authorized"), ShouldEqual, codes.OK)
		So(check(ctx, "mixed", "unknown"), ShouldEqual, codes.PermissionDenied)
	})
}
