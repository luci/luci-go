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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/server/auth"
)

// XSRFTokenMetadataKey is the gRPC metadata key with the XSRF token.
const XSRFTokenMetadataKey = "x-xsrf-token"

// Interceptor returns a server interceptor that check the XSRF token if the
// call was authenticated through the given method (usually some sort of
// cookie-based authentication).
//
// The token should be in the incoming metadata at "x-xsrf-token" key.
//
// This is useful as a defense in depth against unauthorized cross-origin
// requests when using pRPC APIs with cookie-based authentication. Theoretically
// CORS policies and SameSite cookies can also solve this problem, but their
// semantics is pretty complicated and it is easy to mess up.
func Interceptor(method auth.Method) grpcutil.UnifiedServerInterceptor {
	return func(ctx context.Context, _ string, handler func(ctx context.Context) error) (err error) {
		if auth.GetState(ctx).Method() == method {
			found := false
			md, _ := metadata.FromIncomingContext(ctx)
			for _, val := range md[XSRFTokenMetadataKey] {
				err := Check(ctx, val)
				if err == nil {
					found = true
					break
				}
				if transient.Tag.In(err) {
					logging.Errorf(ctx, "Transient error when checking XSRF token: %s", err)
					return status.Errorf(codes.Internal, "internal error when checking XSRF token")
				}
			}
			if !found {
				return status.Errorf(codes.Unauthenticated, "missing or invalid XSRF token in %q metadata", XSRFTokenMetadataKey)
			}
		}
		return handler(ctx)
	}
}
