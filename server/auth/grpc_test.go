// Copyright 2023 The LUCI Authors.
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

package auth

import (
	"context"
	"net"
	"net/http"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	"go.chromium.org/luci/auth/identity"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAuthenticatingInterceptor(t *testing.T) {
	t.Parallel()

	Convey("With request", t, func() {
		ctx := injectTestDB(context.Background(), &fakeDB{
			allowedClientID: "some_client_id",
		})

		requestCtx := metadata.NewIncomingContext(ctx, metadata.MD{
			":authority": {"some.example.com"},
			"x-boom":     {"val1", "val2"},
			// Can happen when using pRPC transport.
			"cookie": {
				"cookie_1=value_1; cookie_2=value_2",
				"cookie_3=value_3",
			},
		})
		requestCtx = peer.NewContext(requestCtx, &peer.Peer{
			Addr: &net.IPAddr{
				IP: net.IPv4(1, 2, 3, 4),
			},
		})

		Convey("Pass", func() {
			intr := AuthenticatingInterceptor([]Method{
				fakeAuthMethod{
					clientID: "some_client_id",
					email:    "someone@example.com",
					observe: func(r RequestMetadata) {
						So(r.Header("X-Boom"), ShouldEqual, "val1")

						cookie, err := r.Cookie("cookie_1")
						So(err, ShouldBeNil)
						So(cookie.Value, ShouldEqual, "value_1")

						cookie, err = r.Cookie("cookie_2")
						So(err, ShouldBeNil)
						So(cookie.Value, ShouldEqual, "value_2")

						cookie, err = r.Cookie("cookie_3")
						So(err, ShouldBeNil)
						So(cookie.Value, ShouldEqual, "value_3")

						_, err = r.Cookie("cookie_4")
						So(err, ShouldEqual, http.ErrNoCookie)

						So(r.RemoteAddr(), ShouldEqual, "1.2.3.4")
						So(r.Host(), ShouldEqual, "some.example.com")
					},
				},
			})

			var innerCtx context.Context
			_, err := intr.Unary()(requestCtx, "request", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
				innerCtx = ctx
				return "response", nil
			})
			So(err, ShouldBeNil)

			// The handler was actually called and with the correct state.
			So(innerCtx, ShouldNotBeNil)
			So(CurrentIdentity(innerCtx), ShouldEqual, identity.Identity("user:someone@example.com"))
		})

		Convey("Blocked", func() {
			intr := AuthenticatingInterceptor([]Method{
				fakeAuthMethod{
					clientID: "another_client_id",
				},
			})
			_, err := intr.Unary()(requestCtx, "request", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
				panic("must not be called")
			})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		})
	})
}
