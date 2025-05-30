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

package tsmon

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/grpc/grpcmon"

	"go.chromium.org/luci/server/auth"
)

// NewProdXMonitor creates a monitor that flushes metrics to the ProdX endpoint.
//
// It splits them into chunks of given size and impersonates the given service
// account to authenticate calls.
func NewProdXMonitor(ctx context.Context, chunkSize int, account string) (monitor.Monitor, error) {
	cred, err := auth.GetPerRPCCredentials(
		ctx, auth.AsActor,
		auth.WithServiceAccount(account),
		auth.WithScopes(monitor.ProdXMonScope),
	)
	if err != nil {
		return nil, errors.Fmt("failed to get per RPC credentials: %w", err)
	}
	conn, err := grpc.NewClient(
		prodXEndpoint,
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(cred),
		grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{}),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, errors.Fmt("failed to dial ProdX service %s: %w", prodXEndpoint, err)
	}
	return monitor.NewGRPCMonitor(ctx, chunkSize, conn), nil
}
