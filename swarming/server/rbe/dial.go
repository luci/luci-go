// Copyright 2023 The LUCI Authors.
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

package rbe

import (
	"context"
	"fmt"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
)

// Instructs the gRPC client how to retry.
var retryPolicy = fmt.Sprintf(`{
  "methodConfig": [{
    "name": [
      {"service": "%s"},
      {"service": "%s"}
    ],
    "waitForReady": true,
    "retryPolicy": {
      "MaxAttempts": 5,
      "InitialBackoff": "0.01s",
      "MaxBackoff": "1s",
      "BackoffMultiplier": 2.0,
      "RetryableStatusCodes": [
        "INTERNAL",
        "UNAVAILABLE"
      ]
    }
  }]
}`,
	remoteworkers.Bots_ServiceDesc.ServiceName,
	remoteworkers.Reservations_ServiceDesc.ServiceName,
)

// Dial dials RBE backend with proper authentication.
//
// Returns multiple identical clients each representing a separate HTTP2
// connection.
func Dial(ctx context.Context, count int) ([]grpc.ClientConnInterface, error) {
	creds, err := auth.GetPerRPCCredentials(ctx,
		auth.AsSelf,
		auth.WithScopes(auth.CloudOAuthScopes...),
	)
	if err != nil {
		return nil, errors.Fmt("failed to get credentials: %w", err)
	}
	logging.Infof(ctx, "Dialing %d RBE backend connections...", count)
	conns := make([]grpc.ClientConnInterface, count)
	for i := 0; i < count; i++ {
		conns[i], err = grpc.NewClient("remotebuildexecution.googleapis.com:443",
			grpc.WithTransportCredentials(credentials.NewTLS(nil)),
			grpc.WithPerRPCCredentials(creds),
			grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{}),
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
			grpc.WithDefaultServiceConfig(retryPolicy),
		)
		if err != nil {
			return nil, errors.Fmt("failed to dial RBE backend: %w", err)
		}
	}
	return conns, nil
}
