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
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcmon"
	executorpb "go.chromium.org/turboci/proto/go/graph/executor/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
	orchestratorgrpcpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1/grpcpb"
)

var orchestratorClientKey = "holds the global turboci orchestrator client"

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

// WithTurboCIOrchestratorClient returns a new context with the given orchestrator client.
func WithTurboCIOrchestratorClient(ctx context.Context, client orchestratorgrpcpb.TurboCIOrchestratorClient) context.Context {
	return context.WithValue(ctx, &orchestratorClientKey, client)
}

// TurboCIOrchestratorClient returns the orchestrator client installed in the current context.
// Panics if there isn't one.
func TurboCIOrchestratorClient(ctx context.Context) orchestratorgrpcpb.TurboCIOrchestratorClient {
	return ctx.Value(&orchestratorClientKey).(orchestratorgrpcpb.TurboCIOrchestratorClient)
}

// CreateWorkPlan is a helper function to get the installed orchestrator client
// and call CreateWorkPlan with it.
func CreateWorkPlan(ctx context.Context, in *orchestratorpb.CreateWorkPlanRequest, opts ...grpc.CallOption) (*orchestratorpb.CreateWorkPlanResponse, error) {
	LogRequest(ctx, "CreateWorkPlan", in)
	resp, err := TurboCIOrchestratorClient(ctx).CreateWorkPlan(ctx, in, opts...)
	LogResponse(ctx, "CreateWorkPlan", resp, err)
	return resp, err
}

// WriteNodes is a helper function to get the installed orchestrator client
// and call WriteNodes with it.
func WriteNodes(ctx context.Context, in *orchestratorpb.WriteNodesRequest, opts ...grpc.CallOption) (*orchestratorpb.WriteNodesResponse, error) {
	LogRequest(ctx, "WriteNodes", in)
	resp, err := TurboCIOrchestratorClient(ctx).WriteNodes(ctx, in, opts...)
	LogResponse(ctx, "WriteNodes", resp, err)
	return resp, err
}

// QueryNodes is a helper function to get the installed orchestrator client
// and call QueryNodes with it.
func QueryNodes(ctx context.Context, in *orchestratorpb.QueryNodesRequest, opts ...grpc.CallOption) (*orchestratorpb.QueryNodesResponse, error) {
	LogRequest(ctx, "QueryNodes", in)
	resp, err := TurboCIOrchestratorClient(ctx).QueryNodes(ctx, in, opts...)
	LogResponse(ctx, "QueryNodes", resp, err)
	return resp, err
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

// LogRequest dumps a slightly redacted Turbo CI request proto into the log.
func LogRequest(ctx context.Context, rpc string, req any) {
	logging.Infof(ctx, "turbo-ci: %s request:\n%s", rpc, turboCIMsgToText(req))
}

// LogResponse dumps a slightly redacted Turbo CI response proto into the log.
func LogResponse(ctx context.Context, rpc string, resp any, err error) {
	if err != nil {
		logging.Warningf(ctx, "turbo-ci: %s response: %s", rpc, err)
	} else {
		logging.Infof(ctx, "turbo-ci: %s response:\n%s", rpc, turboCIMsgToText(resp))
	}
}

// turboCIMsgToText redacts the token and logs the message.
func turboCIMsgToText(msg any) string {
	if msg == nil {
		return "nil"
	}
	m, _ := msg.(proto.Message)
	if m == nil {
		return fmt.Sprintf("<not a proto: %T>", msg)
	}
	switch v := m.(type) {
	case *orchestratorpb.WriteNodesRequest:
		defer redactToken(v.HasToken, v.GetToken, v.SetToken)()
	case *orchestratorpb.QueryNodesRequest:
		defer redactToken(v.HasToken, v.GetToken, v.SetToken)()
	case *orchestratorpb.CreateWorkPlanResponse:
		defer redactToken(v.HasCreatorToken, v.GetCreatorToken, v.SetCreatorToken)()
	case *executorpb.RunStageRequest:
		defer redactToken(v.HasStageAttemptToken, v.GetStageAttemptToken, v.SetStageAttemptToken)()
	}
	blob, err := (prototext.MarshalOptions{Multiline: true}).Marshal(m)
	if err != nil {
		return fmt.Sprintf("<marshal error %q>", err)
	}
	if len(blob) == 0 {
		return "<empty>"
	}
	return string(blob)
}

func redactToken(has func() bool, get func() string, set func(string)) func() {
	if !has() {
		return func() {}
	}
	val := get()
	set(strings.Repeat("x", len(val)))
	return func() { set(val) }
}
