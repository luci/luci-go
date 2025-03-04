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
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
)

type grpcRequestMetadata struct {
	ctx context.Context
}

func (m grpcRequestMetadata) Header(key string) string {
	if v := metadata.ValueFromIncomingContext(m.ctx, strings.ToLower(key)); len(v) != 0 {
		return v[0]
	}
	return ""
}

func (m grpcRequestMetadata) Cookie(key string) (*http.Cookie, error) {
	// Note: this is really used only when the transport is pRPC and requests come
	// from a browser.
	cookies := metadata.ValueFromIncomingContext(m.ctx, "cookie")
	if len(cookies) == 0 {
		return nil, http.ErrNoCookie
	}
	return (&http.Request{Header: http.Header{"Cookie": cookies}}).Cookie(key)
}

func (m grpcRequestMetadata) RemoteAddr() string {
	if peer, ok := peer.FromContext(m.ctx); ok {
		return peer.Addr.String()
	}
	return ""
}

func (m grpcRequestMetadata) Host() string {
	if v := metadata.ValueFromIncomingContext(m.ctx, ":authority"); len(v) != 0 {
		return v[0]
	}
	return ""
}

// RequestMetadataForGRPC returns a RequestMetadata of the current grpc request.
func RequestMetadataForGRPC(ctx context.Context) RequestMetadata {
	return grpcRequestMetadata{ctx}
}

// AuthenticatingInterceptor authenticates incoming requests.
//
// It performs per-RPC authentication, i.e. it is a server counterpart of the
// client side PerRPCCredentials option. It doesn't do any transport-level
// authentication.
//
// It receives a list of Method implementations which will be applied one after
// another to try to authenticate the request until the first successful hit. If
// all methods end up to be non-applicable (i.e. none of the methods notice any
// metadata they recognize), the request will be passed through to the handler
// as anonymous (coming from an "anonymous identity"). Rejecting anonymous
// requests (if necessary) is the job of an authorization layer, often
// implemented as a separate gRPC interceptor. For simple cases use
// go.chromium.org/luci/server/auth/rpcacl interceptor.
//
// Additionally this interceptor adds an authentication state into the request
// context. It is used by various functions in this package such as
// CurrentIdentity and HasPermission.
//
// The context in the incoming request should be derived from a context that
// holds at least the auth library configuration (see Config and ModifyConfig),
// but ideally also other foundational things (like logging, monitoring, etc).
// This is usually already the case when running in a LUCI Server environment.
//
// May abort the request without calling the handler if the authentication
// process itself fails in some way. In particular:
//   - PERMISSION_DENIED: a forbidden client IP or a token audience.
//   - UNAUTHENTICATED: present, but totally malformed authorization metadata.
//   - INTERNAL: some transient error.
func AuthenticatingInterceptor(methods []Method) grpcutil.UnifiedServerInterceptor {
	au := Authenticator{Methods: methods}
	return func(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) error {
		ctx, err := au.Authenticate(ctx, RequestMetadataForGRPC(ctx))
		if err != nil {
			code, ok := grpcutil.Tag.Value(err)
			if !ok {
				if transient.Tag.In(err) {
					code = codes.Internal
				} else {
					code = codes.Unauthenticated
				}
			}
			return status.Error(code, err.Error())
		}
		return handler(ctx)
	}
}
