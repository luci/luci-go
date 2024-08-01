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

// Package rpcutil contains utility functions for RPC handlers.
package rpcutil

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/auth"
)

// IdentityKindCounter is exported for testing.
//
// Normally this counter is updated via IdentityKindCountingInterceptor().
var IdentityKindCounter = metric.NewCounter(
	"resultdb/rpc/identitykind",
	"Number of identities",
	nil,
	field.String("service"), // RPC service
	field.String("method"),  // RPC method
	field.String("kind"),    // LUCI auth identity Kind
)

// IdentityKindCountingInterceptor returns a gRPC interceptor that updates a
// tsmon counter with the type of authenticated principal.
func IdentityKindCountingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		// FullMethod has form "/<service>/<method>".
		parts := strings.Split(info.FullMethod, "/")
		if len(parts) != 3 || parts[0] != "" {
			panic(fmt.Sprintf("unexpected format of info.FullMethod: %q", info.FullMethod))
		}
		service, method := parts[1], parts[2]

		IdentityKindCounter.Add(ctx, 1, service, method, string(auth.CurrentIdentity(ctx).Kind()))
		return handler(ctx, req)
	}
}

// RequestTimeoutInterceptor returns a gRPC interceptor that set context timeout based on the RPC name.
func RequestTimeoutInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		if info.FullMethod == "/luci.resultdb.v1.ResultDB/QueryTestVariantArtifactGroups" {
			return handler(ctx, req)
		}
		// All other RPCs should have a timeout limit of 1 minute.
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		return handler(ctx, req)
	}
}
