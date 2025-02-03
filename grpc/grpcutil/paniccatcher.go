// Copyright 2017 The LUCI Authors.
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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/runtime/paniccatcher"
)

// PanicCatcherInterceptor is a UnifiedServerInterceptor that catches panics
// in RPC handlers, recovers them and returns codes.Internal gRPC errors
// instead.
func PanicCatcherInterceptor(ctx context.Context, fullMethod string, handler func(ctx context.Context) error) (err error) {
	defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
		p.Log(ctx, "Panic handling %q: %s", fullMethod, p.Reason)
		err = status.Error(codes.Internal, "panic in the request handler")
	})
	return handler(ctx)
}

var (
	// UnaryServerPanicCatcherInterceptor is a grpc.UnaryServerInterceptor that
	// catches panics in RPC handlers, recovers them and returns codes.Internal gRPC
	// errors instead.
	UnaryServerPanicCatcherInterceptor = UnifiedServerInterceptor(PanicCatcherInterceptor).Unary()

	// StreamServerPanicCatcherInterceptor is a grpc.StreamServerInterceptor that
	// catches panics in RPC handlers, recovers them and returns codes.Internal gRPC
	// errors instead.
	StreamServerPanicCatcherInterceptor = UnifiedServerInterceptor(PanicCatcherInterceptor).Stream()
)
