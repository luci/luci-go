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

// Package swarming contains the Swarming client.
//
// It is a "fat" client that does extra things on top of just wrapping the
// server API.
package swarming

import (
	"context"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

const (
	// ServerEnvVar is Swarming server host to which a client connects.
	ServerEnvVar = "SWARMING_SERVER"

	// TaskIDEnvVar is a Swarming task ID in which this task is running.
	//
	// The `swarming` command line tool uses this to populate `ParentTaskId`
	// when being used to trigger new tasks from within a swarming task.
	TaskIDEnvVar = "SWARMING_TASK_ID"

	// UserEnvVar is the OS user name (not Swarming specific).
	//
	// The `swarming` command line tool uses this to populate `User`
	// when being used to trigger new tasks.
	UserEnvVar = "USER"
)

// Swarming represents the Swarming service API.
type Swarming interface {
	NewTask(ctx context.Context, req *swarmingv2.NewTaskRequest) (*swarmingv2.TaskRequestMetadataResponse, error)
	CountTasks(ctx context.Context, start float64, state swarmingv2.StateQuery, tags ...string) (*swarmingv2.TasksCount, error)
	ListTasks(ctx context.Context, limit int32, start float64, state swarmingv2.StateQuery, tags []string) ([]*swarmingv2.TaskResultResponse, error)
	CancelTask(ctx context.Context, taskID string, killRunning bool) (*swarmingv2.CancelResponse, error)
	TaskRequest(ctx context.Context, taskID string) (*swarmingv2.TaskRequestResponse, error)
	TaskOutput(ctx context.Context, taskID string) (*swarmingv2.TaskOutputResponse, error)
	TaskResult(ctx context.Context, taskID string, perf bool) (*swarmingv2.TaskResultResponse, error)
	CountBots(ctx context.Context, dimensions []*swarmingv2.StringPair) (*swarmingv2.BotsCount, error)
	ListBots(ctx context.Context, dimensions []*swarmingv2.StringPair) ([]*swarmingv2.BotInfo, error)
	DeleteBot(ctx context.Context, botID string) (*swarmingv2.DeleteResponse, error)
	TerminateBot(ctx context.Context, botID string, reason string) (*swarmingv2.TerminateResponse, error)
	ListBotTasks(ctx context.Context, botID string, limit int32, start float64, state swarmingv2.StateQuery) ([]*swarmingv2.TaskResultResponse, error)
	FilesFromCAS(ctx context.Context, outdir string, cascli *rbeclient.Client, casRef *swarmingv2.CASReference) ([]string, error)
}
