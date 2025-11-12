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

// Package turboci contains helpers for calling Turbo CI Orchestrator.
package turboci

import (
	"context"
	"fmt"
	"math/rand/v2"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcmon"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
	orchestratorgrpcpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1/grpcpb"
)

// How many gRPC clients to use to avoid hitting HTTP2 concurrent stream limits.
const connPoolSize = 4

// Instructs the gRPC client how to retry.
var retryPolicy = fmt.Sprintf(`{
  "methodConfig": [{
    "name": [
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
	orchestratorgrpcpb.TurboCIOrchestrator_ServiceDesc.ServiceName,
)

// Dial establishes a connection to the Turbo CI Orchestrator.
//
// Each individual call must supply its own per-RPC credentials, there's no
// default in the client.
func Dial(ctx context.Context, apiEndpoint string) (orchestratorgrpcpb.TurboCIOrchestratorClient, error) {
	logging.Infof(ctx, "Dialing Turbo CI at %q...", apiEndpoint)
	pool := make(clientPool, connPoolSize)
	for i := range connPoolSize {
		var err error
		pool[i], err = grpc.NewClient(apiEndpoint,
			grpc.WithTransportCredentials(credentials.NewTLS(nil)),
			grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{}),
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
			grpc.WithDefaultServiceConfig(retryPolicy),
		)
		if err != nil {
			return nil, errors.Fmt("failed to dial Turbo CI Orchestrator: %w", err)
		}
	}
	return pool, nil
}

type clientPool []grpc.ClientConnInterface

// CreateWorkPlan implements TurboCIOrchestratorClient.
func (cp clientPool) CreateWorkPlan(ctx context.Context, in *orchestratorpb.CreateWorkPlanRequest, opts ...grpc.CallOption) (*orchestratorpb.CreateWorkPlanResponse, error) {
	return cp.pickRandom().CreateWorkPlan(ctx, in, opts...)
}

// WriteNodes implements TurboCIOrchestratorClient.
func (cp clientPool) WriteNodes(ctx context.Context, in *orchestratorpb.WriteNodesRequest, opts ...grpc.CallOption) (*orchestratorpb.WriteNodesResponse, error) {
	return cp.pickRandom().WriteNodes(ctx, in, opts...)
}

// QueryNodes implements TurboCIOrchestratorClient.
func (cp clientPool) QueryNodes(ctx context.Context, in *orchestratorpb.QueryNodesRequest, opts ...grpc.CallOption) (*orchestratorpb.QueryNodesResponse, error) {
	return cp.pickRandom().QueryNodes(ctx, in, opts...)
}

// pickRandom returns a random connection from the pool.
func (cp clientPool) pickRandom() orchestratorgrpcpb.TurboCIOrchestratorClient {
	return orchestratorgrpcpb.NewTurboCIOrchestratorClient(cp[rand.IntN(len(cp))])
}
