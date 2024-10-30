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
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestAuthenticatingInterceptor(t *testing.T) {
	t.Parallel()

	ftt.Run("With request", t, func(t *ftt.Test) {
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

		t.Run("Pass", func(t *ftt.Test) {
			intr := AuthenticatingInterceptor([]Method{
				fakeAuthMethod{
					clientID: "some_client_id",
					email:    "someone@example.com",
					observe: func(r RequestMetadata) {
						assert.Loosely(t, r.Header("X-Boom"), should.Equal("val1"))

						cookie, err := r.Cookie("cookie_1")
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, cookie.Value, should.Equal("value_1"))

						cookie, err = r.Cookie("cookie_2")
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, cookie.Value, should.Equal("value_2"))

						cookie, err = r.Cookie("cookie_3")
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, cookie.Value, should.Equal("value_3"))

						_, err = r.Cookie("cookie_4")
						assert.Loosely(t, err, should.Equal(http.ErrNoCookie))

						assert.Loosely(t, r.RemoteAddr(), should.Equal("1.2.3.4"))
						assert.Loosely(t, r.Host(), should.Equal("some.example.com"))
					},
				},
			})

			var innerCtx context.Context
			_, err := intr.Unary()(requestCtx, "request", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
				innerCtx = ctx
				return "response", nil
			})
			assert.Loosely(t, err, should.BeNil)

			// The handler was actually called and with the correct state.
			assert.Loosely(t, innerCtx, should.NotBeNil)
			assert.Loosely(t, CurrentIdentity(innerCtx), should.Equal(identity.Identity("user:someone@example.com")))
		})

		t.Run("Blocked", func(t *ftt.Test) {
			intr := AuthenticatingInterceptor([]Method{
				fakeAuthMethod{
					clientID: "another_client_id",
				},
			})
			_, err := intr.Unary()(requestCtx, "request", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
				panic("must not be called")
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})
	})
}
