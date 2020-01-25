// Copyright 2020 The LUCI Authors.
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

package grpcutil

import (
	"context"

	"google.golang.org/grpc"
)

// ChainUnaryServerInterceptors chains multiple interceptors together.
//
// The first one becomes the outermost, and the last one becomes the
// innermost, i.e. `ChainUnaryServerInterceptors(a, b, c)(h) === a(b(c(h)))`.
func ChainUnaryServerInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	switch len(interceptors) {
	case 0:
		return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	case 1:
		return interceptors[0]
	default:
		return combinator(interceptors[0], ChainUnaryServerInterceptors(interceptors[1:]...))
	}
}

// combinator is an interceptor that chains just two interceptors together.
func combinator(first, second grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return first(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
			return second(ctx, req, info, handler)
		})
	}
}
