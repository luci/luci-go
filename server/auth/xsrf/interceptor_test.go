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

package xsrf

import (
	"context"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/discovery"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
)

const (
	fakeIdentity = identity.Identity("user:someone@example.com")
	authKey      = "Fake-Auth-Header"
)

func TestInterceptor(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := makeContext()
		ctx = authtest.MockAuthConfig(ctx)

		authMethod1 := &testAuthMethod{expect: "1"}
		authMethod2 := &testAuthMethod{expect: "2"}

		rpcSrv := &prpc.Server{
			// Install the interceptor that checks the token **only** when authMethod1
			// is used.
			UnaryServerInterceptor: grpcutil.ChainUnaryServerInterceptors(
				auth.AuthenticatingInterceptor([]auth.Method{authMethod1, authMethod2}).Unary(),
				Interceptor(authMethod1).Unary(),
			),
		}

		// We need some API to call in the test. Reuse Discovery API for that since
		// it is simple enough and have no side effects.
		discovery.Enable(rpcSrv)

		// Expose pRPC API via test HTTP server.
		router := router.New()
		rpcSrv.InstallHandlers(router, nil)
		httpSrv := httptest.NewUnstartedServer(router)
		httpSrv.Config.BaseContext = func(net.Listener) context.Context { return ctx }
		httpSrv.Start()
		defer httpSrv.Close()

		// Create a discovery client targeting our test server.
		apiClient := discovery.NewDiscoveryPRPCClient(&prpc.Client{
			Host: strings.TrimPrefix(httpSrv.URL, "http://"),
			Options: &prpc.Options{
				Insecure: true, // not using HTTPS
			},
		})

		call := func(md ...string) codes.Code {
			ctx := metadata.NewOutgoingContext(ctx, metadata.Pairs(md...))
			_, err := apiClient.Describe(ctx, &discovery.Void{})
			return status.Code(err)
		}

		goodToken, err := Token(auth.WithState(ctx, &authtest.FakeState{
			Identity: fakeIdentity,
		}))
		assert.Loosely(t, err, should.BeNil)

		badToken, err := Token(auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone-else@example.com",
		}))
		assert.Loosely(t, err, should.BeNil)

		// A token is checked, and only when using method "1".
		assert.Loosely(t, call(authKey, "1", XSRFTokenMetadataKey, goodToken), should.Equal(codes.OK))
		assert.Loosely(t, call(authKey, "1", XSRFTokenMetadataKey, "", XSRFTokenMetadataKey, goodToken), should.Equal(codes.OK))
		assert.Loosely(t, call(authKey, "1", XSRFTokenMetadataKey, badToken), should.Equal(codes.Unauthenticated))
		assert.Loosely(t, call(authKey, "1", XSRFTokenMetadataKey, ""), should.Equal(codes.Unauthenticated))
		assert.Loosely(t, call(authKey, "1"), should.Equal(codes.Unauthenticated))

		// When using method "2" the token is ignored.
		assert.Loosely(t, call(authKey, "2", XSRFTokenMetadataKey, goodToken), should.Equal(codes.OK))
		assert.Loosely(t, call(authKey, "2", XSRFTokenMetadataKey, badToken), should.Equal(codes.OK))
		assert.Loosely(t, call(authKey, "2", XSRFTokenMetadataKey, ""), should.Equal(codes.OK))
		assert.Loosely(t, call(authKey, "2"), should.Equal(codes.OK))
	})
}

type testAuthMethod struct {
	expect string
}

func (t *testAuthMethod) Authenticate(ctx context.Context, req auth.RequestMetadata) (*auth.User, auth.Session, error) {
	if req.Header(authKey) == t.expect {
		return &auth.User{Identity: fakeIdentity}, nil, nil
	}
	return nil, nil, nil // skip this method
}
