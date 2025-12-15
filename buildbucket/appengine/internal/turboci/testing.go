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

package turboci

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// FakeOrchestratorClient is a faked TurboCIOrchestratorClient for testing.
type FakeOrchestratorClient struct {
	// LastCreateCall is the last CreateWorkPlanRequest.
	LastCreateCall *orchestratorpb.CreateWorkPlanRequest
	// LastWriteNodesCall is the last WriteNodesRequest.
	LastWriteNodesCall *orchestratorpb.WriteNodesRequest
	// LastQueryNodesRequest is the last QueryNodesRequest.
	LastQueryNodesRequest *orchestratorpb.QueryNodesRequest

	// PlanID is the ID of the workplan to create.
	PlanID string
	// Token is the token to use.
	Token string

	QueryNodesResponse *orchestratorpb.QueryNodesResponse
	Err                error
}

// CreateWorkPlan implements TurboCIOrchestratorClient.
func (o *FakeOrchestratorClient) CreateWorkPlan(ctx context.Context, in *orchestratorpb.CreateWorkPlanRequest, opts ...grpc.CallOption) (*orchestratorpb.CreateWorkPlanResponse, error) {
	o.LastCreateCall = proto.Clone(in).(*orchestratorpb.CreateWorkPlanRequest)
	return orchestratorpb.CreateWorkPlanResponse_builder{
		Identifier: idspb.WorkPlan_builder{
			Id: proto.String(o.PlanID),
		}.Build(),
		CreatorToken: proto.String(o.Token),
	}.Build(), nil
}

// WriteNodes implements TurboCIOrchestratorClient.
func (o *FakeOrchestratorClient) WriteNodes(ctx context.Context, in *orchestratorpb.WriteNodesRequest, opts ...grpc.CallOption) (*orchestratorpb.WriteNodesResponse, error) {
	o.LastWriteNodesCall = proto.Clone(in).(*orchestratorpb.WriteNodesRequest)
	return &orchestratorpb.WriteNodesResponse{}, nil
}

// QueryNodes implements TurboCIOrchestratorClient.
func (o *FakeOrchestratorClient) QueryNodes(ctx context.Context, in *orchestratorpb.QueryNodesRequest, opts ...grpc.CallOption) (*orchestratorpb.QueryNodesResponse, error) {
	o.LastQueryNodesRequest = proto.Clone(in).(*orchestratorpb.QueryNodesRequest)
	return o.QueryNodesResponse, o.Err
}
