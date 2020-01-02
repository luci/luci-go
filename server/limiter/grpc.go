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

package limiter

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewUnaryServerInterceptor returns an interceptor that uses the given limiter
// to accept or drop gRPC requests.
func NewUnaryServerInterceptor(l *Limiter, next grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		done, err := l.CheckRequest(ctx, &RequestInfo{
			CallLabel: info.FullMethod,
			PeerLabel: PeerLabelFromAuthState(ctx),
		})
		if err != nil {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		defer done()
		if next != nil {
			return next(ctx, req, info, handler)
		}
		return handler(ctx, req)
	}
}
