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

package clients

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	apigrpcpb "go.chromium.org/luci/swarming/proto/api_v2/grpcpb"
)

// SwarmingClient is a Swarming API wrapper for buildbucket-specific usage.
//
// In prod, a SwarmingClient for interacting with the Swarming service will be
// used. Tests should use a fake implementation.
type SwarmingClient interface {
	CreateTask(ctx context.Context, createTaskReq *apipb.NewTaskRequest) (*apipb.TaskRequestMetadataResponse, error)
	GetTaskResult(ctx context.Context, taskID string) (*apipb.TaskResultResponse, error)
	CancelTask(ctx context.Context, req *apipb.TaskCancelRequest) (*apipb.CancelResponse, error)
}

// swarmingServiceImpl for use in real production envs.
type swarmingServiceImpl struct {
	TasksClient apigrpcpb.TasksClient
}

// Ensure swarmingClientImpl implements SwarmingClient.
var _ SwarmingClient = &swarmingServiceImpl{}

var MockSwarmingClientKey = "used in tests only for setting the mock SwarmingClient"

// NewSwarmingClient returns a new SwarmingClient to interact with Swarming APIs.
func NewSwarmingClient(ctx context.Context, host string, project string) (SwarmingClient, error) {
	if mockClient, ok := ctx.Value(&MockSwarmingClientKey).(*MockSwarmingClient); ok {
		return mockClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project))
	if err != nil {
		return nil, err
	}

	prpcClient := prpc.Client{
		C:    &http.Client{Transport: t},
		Host: host,
	}
	return &swarmingServiceImpl{
		TasksClient: apigrpcpb.NewTasksClient(&prpcClient),
	}, nil
}

// CreateTask calls `apigrpcpb.TasksClient.NewTask` to create a task.
func (s *swarmingServiceImpl) CreateTask(ctx context.Context, createTaskReq *apipb.NewTaskRequest) (*apipb.TaskRequestMetadataResponse, error) {
	subCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.TasksClient.NewTask(subCtx, createTaskReq)
}

// GetTaskResult calls `apigrpcpb.TasksClient.GetResult` to get the result of a task via a task id.
func (s *swarmingServiceImpl) GetTaskResult(ctx context.Context, taskID string) (*apipb.TaskResultResponse, error) {
	subCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.TasksClient.GetResult(subCtx, &apipb.TaskIdWithPerfRequest{TaskId: taskID})
}

func (s *swarmingServiceImpl) CancelTask(ctx context.Context, req *apipb.TaskCancelRequest) (*apipb.CancelResponse, error) {
	subCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.TasksClient.CancelTask(subCtx, req)
}
