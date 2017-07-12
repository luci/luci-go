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
