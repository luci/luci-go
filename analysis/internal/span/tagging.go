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

package span

import (
	"context"

	"google.golang.org/grpc"

	"go.chromium.org/luci/server/span"
)

// SpannerDefaultsInterceptor returns a gRPC interceptor that adds default
// Spanner request options to the context.
//
// The request tag will be set the to RPC method name.
//
// See also ModifyRequestOptions in luci/server/span.
func SpannerDefaultsInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		ctx = span.ModifyRequestOptions(ctx, func(opts *span.RequestOptions) {
			opts.Tag = info.FullMethod
		})
		return handler(ctx, req)
	}
}
