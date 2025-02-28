// Copyright 2025 The LUCI Authors.
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

package proxyserver

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
)

// UnimplementedProxyInterceptor amends Unimplemented error messages to indicate
// that it is the proxy that doesn't implement RPCs (not the remote backend).
//
// There's a very small chance the remote is not implementing an RPC. We ignore
// this possibility.
func UnimplementedProxyInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if status.Code(err) == codes.Unimplemented {
			err = status.Errorf(codes.Unimplemented, "CIPD proxy: %s", status.Convert(err).Message())
		}
		return resp, err
	}
}

// AccessLogInterceptor reports all handled RPCs to the given callback.
func AccessLogInterceptor(cb func(ctx context.Context, entry *cipdpb.AccessLogEntry)) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// TODO: More details.
		resp, err := handler(ctx, req)
		cb(ctx, &cipdpb.AccessLogEntry{
			Method: info.FullMethod,
		})
		return resp, err
	}
}
