// Copyright 2017 The LUCI Authors.
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

package swarmingimpl

import (
	"context"
	"flag"
	"net/http"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"

	swarmingv1 "go.chromium.org/luci/common/api/swarming/swarming/v1"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
	"google.golang.org/api/googleapi"
)

var _ swarmingService = (*testService)(nil)

type testService struct {
	newTask      func(context.Context, *swarmingv1.SwarmingRpcsNewTaskRequest) (*swarmingv1.SwarmingRpcsTaskRequestMetadata, error)
	countTasks   func(context.Context, float64, string, ...string) (*swarmingv1.SwarmingRpcsTasksCount, error)
	countBots    func(context.Context, []*swarmingv2.StringPair) (*swarmingv2.BotsCount, error)
	listTasks    func(context.Context, int64, float64, string, []string, []googleapi.Field) ([]*swarmingv1.SwarmingRpcsTaskResult, error)
	cancelTask   func(context.Context, string, bool) (*swarmingv2.CancelResponse, error)
	taskRequest  func(context.Context, string) (*swarmingv2.TaskRequestResponse, error)
	taskResult   func(context.Context, string, bool) (*swarmingv1.SwarmingRpcsTaskResult, error)
	taskOutput   func(context.Context, string) (*swarmingv1.SwarmingRpcsTaskOutput, error)
	filesFromCAS func(context.Context, string, *rbeclient.Client, *swarmingv2.CASReference) ([]string, error)
	listBots     func(context.Context, []*swarmingv2.StringPair) ([]*swarmingv2.BotInfo, error)
	deleteBot    func(context.Context, string) (*swarmingv2.DeleteResponse, error)
	terminateBot func(context.Context, string, string) (*swarmingv2.TerminateResponse, error)
	listBotTasks func(context.Context, string, int64, float64, string, []googleapi.Field) ([]*swarmingv1.SwarmingRpcsTaskResult, error)
}

func (testService) Client() *http.Client {
	return nil
}

func (s testService) NewTask(ctx context.Context, req *swarmingv1.SwarmingRpcsNewTaskRequest) (*swarmingv1.SwarmingRpcsTaskRequestMetadata, error) {
	return s.newTask(ctx, req)
}

func (s testService) CountTasks(ctx context.Context, start float64, state string, tags ...string) (*swarmingv1.SwarmingRpcsTasksCount, error) {
	return s.countTasks(ctx, start, state, tags...)
}

func (s testService) ListTasks(ctx context.Context, limit int64, start float64, state string, tags []string, fields []googleapi.Field) ([]*swarmingv1.SwarmingRpcsTaskResult, error) {
	return s.listTasks(ctx, limit, start, state, tags, fields)
}

func (s testService) CancelTask(ctx context.Context, taskID string, killRunning bool) (*swarmingv2.CancelResponse, error) {
	return s.cancelTask(ctx, taskID, killRunning)
}

func (s testService) TaskRequest(ctx context.Context, taskID string) (*swarmingv2.TaskRequestResponse, error) {
	return s.taskRequest(ctx, taskID)
}

func (s testService) TaskResult(ctx context.Context, taskID string, perf bool) (*swarmingv1.SwarmingRpcsTaskResult, error) {
	return s.taskResult(ctx, taskID, perf)
}

func (s testService) TaskOutput(ctx context.Context, taskID string) (*swarmingv1.SwarmingRpcsTaskOutput, error) {
	return s.taskOutput(ctx, taskID)
}

func (s testService) FilesFromCAS(ctx context.Context, outdir string, cascli *rbeclient.Client, casRef *swarmingv2.CASReference) ([]string, error) {
	return s.filesFromCAS(ctx, outdir, cascli, casRef)
}

func (s testService) CountBots(ctx context.Context, dimensions []*swarmingv2.StringPair) (*swarmingv2.BotsCount, error) {
	return s.countBots(ctx, dimensions)
}

func (s *testService) ListBots(ctx context.Context, dimensions []*swarmingv2.StringPair) ([]*swarmingv2.BotInfo, error) {
	return s.listBots(ctx, dimensions)
}

func (s testService) DeleteBot(ctx context.Context, botID string) (*swarmingv2.DeleteResponse, error) {
	return s.deleteBot(ctx, botID)
}

func (s testService) TerminateBot(ctx context.Context, botID string, reason string) (*swarmingv2.TerminateResponse, error) {
	return s.terminateBot(ctx, botID, reason)
}

func (s testService) ListBotTasks(ctx context.Context, botID string, limit int64, start float64, state string, fields []googleapi.Field) ([]*swarmingv1.SwarmingRpcsTaskResult, error) {
	return s.listBotTasks(ctx, botID, limit, start, state, fields)
}

var _ AuthFlags = (*testAuthFlags)(nil)

type testAuthFlags struct{}

func (af *testAuthFlags) Register(_ *flag.FlagSet) {}

func (af *testAuthFlags) Parse() error { return nil }

func (af *testAuthFlags) NewHTTPClient(_ context.Context) (*http.Client, error) {
	return nil, nil
}

func (af *testAuthFlags) NewRBEClient(_ context.Context, _ string, _ string) (*rbeclient.Client, error) {
	return nil, nil
}
