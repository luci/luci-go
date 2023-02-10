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

// ChainUnaryServerInterceptors chains multiple unary interceptors together.
//
// The first one becomes the outermost, and the last one becomes the
// innermost, i.e. `ChainUnaryServerInterceptors(a, b, c)(h) === a(b(c(h)))`.
//
// nil-valued interceptors are silently skipped.
func ChainUnaryServerInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	switch {
	case len(interceptors) == 0:
		// Noop interceptor.
		return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	case interceptors[0] == nil:
		// Skip nils.
		return ChainUnaryServerInterceptors(interceptors[1:]...)
	case len(interceptors) == 1:
		// No need to actually chain anything.
		return interceptors[0]
	default:
		return unaryCombinator(interceptors[0], ChainUnaryServerInterceptors(interceptors[1:]...))
	}
}

// unaryCombinator is an interceptor that chains just two interceptors together.
func unaryCombinator(first, second grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return first(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
			return second(ctx, req, info, handler)
		})
	}
}

// ChainStreamServerInterceptors chains multiple stream interceptors together.
//
// The first one becomes the outermost, and the last one becomes the
// innermost, i.e. `ChainStreamServerInterceptors(a, b, c)(h) === a(b(c(h)))`.
//
// nil-valued interceptors are silently skipped.
func ChainStreamServerInterceptors(interceptors ...grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	switch {
	case len(interceptors) == 0:
		// Noop interceptor.
		return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, ss)
		}
	case interceptors[0] == nil:
		// Skip nils.
		return ChainStreamServerInterceptors(interceptors[1:]...)
	case len(interceptors) == 1:
		// No need to actually chain anything.
		return interceptors[0]
	default:
		return streamCombinator(interceptors[0], ChainStreamServerInterceptors(interceptors[1:]...))
	}
}

// unaryCombinator is an interceptor that chains just two interceptors together.
func streamCombinator(first, second grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return first(srv, ss, info, func(srv interface{}, ss grpc.ServerStream) error {
			return second(srv, ss, info, handler)
		})
	}
}

// ModifyServerStreamContext returns a ServerStream that fully wraps the given
// one except its context is modified based on the result of the given callback.
//
// This is handy when implementing stream server interceptors that need to
// put stuff into the stream's context.
//
// The callback will be called immediately and only once. It must return a
// context derived from the context it receives or nil if the context
// modification is not actually necessary.
func ModifyServerStreamContext(ss grpc.ServerStream, cb func(context.Context) context.Context) grpc.ServerStream {
	original := ss.Context()
	modified := cb(original)
	if modified == nil || modified == original {
		return ss
	}
	return &wrappedSS{ss, modified}
}

// wrappedSS is a grpc.ServerStream that replaces the context.
type wrappedSS struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the context for this stream.
//
// This is part of grpc.ServerStream interface.
func (ss *wrappedSS) Context() context.Context {
	return ss.ctx
}
