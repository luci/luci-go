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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/grpc/grpcutil"
)

// NewServerInterceptor returns a UnifiedServerInterceptor that uses the given
// limiter to accept or drop gRPC requests.
func NewServerInterceptor(l *Limiter) grpcutil.UnifiedServerInterceptor {
	return func(ctx context.Context, fullMethod string, handler func(context.Context) error) error {
		done, err := l.CheckRequest(ctx, &RequestInfo{
			CallLabel: fullMethod,
			PeerLabel: PeerLabelFromAuthState(ctx),
		})
		if err != nil {
			return status.Error(codes.Unavailable, err.Error())
		}
		defer done()
		return handler(ctx)
	}
}
