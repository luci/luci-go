// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package grpcutil

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/runtime/paniccatcher"
)

// NewUnaryServerPanicCatcher returns a unary interceptor that catches panics
// in RPC handlers, recovers them and returns codes.Internal gRPC errors
// instead.
//
// It can be optionally chained with other interceptor.
func NewUnaryServerPanicCatcher(next grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
			logging.Fields{
				"panic.error": p.Reason,
			}.Errorf(ctx, "Caught panic during handling of %q: %s\n%s", info.FullMethod, p.Reason, p.Stack)
			err = Internal
		})
		if next != nil {
			return next(ctx, req, info, handler)
		}
		return handler(ctx, req)
	}
}
