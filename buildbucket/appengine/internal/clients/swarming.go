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
	"fmt"
	"net/http"
	"time"

	"google.golang.org/api/option"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/server/auth"
)

// SwarmingClient is a Swarming API wrapper for buildbucket-specific usage.
//
// In prod, a SwarmingClient for interacting with the Swarming service will be
// used. Tests should use a fake implementation.
type SwarmingClient interface {
	CreateTask(c context.Context, createTaskReq *swarming.SwarmingRpcsNewTaskRequest) (*swarming.SwarmingRpcsTaskRequestMetadata, error)
	GetTaskResult(ctx context.Context, taskID string) (*swarming.SwarmingRpcsTaskResult, error)
	CancelTask(ctx context.Context, taskID string, req *swarming.SwarmingRpcsTaskCancelRequest) (*swarming.SwarmingRpcsCancelResponse, error)
}

// swarmingClientImpl for use in real production envs.
type swarmingClientImpl swarming.Service

// Ensure swarmingClientImpl implements SwarmingClient.
var _ SwarmingClient = &swarmingClientImpl{}

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
	swarmingService, err := swarming.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: t}))
	if err != nil {
		return nil, err
	}
	swarmingService.BasePath = fmt.Sprintf("https://%s/_ah/api/swarming/v1/", host)
	spl := (*swarmingClientImpl)(swarmingService)
	return spl, nil
}

// CreateTask calls `swarming.tasks.new` to create a task.
func (s *swarmingClientImpl) CreateTask(ctx context.Context, createTaskReq *swarming.SwarmingRpcsNewTaskRequest) (*swarming.SwarmingRpcsTaskRequestMetadata, error) {
	subCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.Tasks.New(createTaskReq).Context(subCtx).Do()
}

// GetTaskResult calls `swarming.task.result` to get the result of a task via a task id.
func (s *swarmingClientImpl) GetTaskResult(ctx context.Context, taskID string) (*swarming.SwarmingRpcsTaskResult, error) {
	subCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.Task.Result(taskID).Context(subCtx).Do()
}

func (s *swarmingClientImpl) CancelTask(ctx context.Context, taskID string, req *swarming.SwarmingRpcsTaskCancelRequest) (*swarming.SwarmingRpcsCancelResponse, error) {
	subCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.Task.Cancel(taskID, req).Context(subCtx).Do()
}
