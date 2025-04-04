// Copyright 2024 The LUCI Authors.
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

package rpcs

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/tasks"
)

// CancelTask implements the corresponding RPC method.
func (srv *TasksServer) CancelTask(ctx context.Context, req *apipb.TaskCancelRequest) (*apipb.CancelResponse, error) {
	taskRequest, err := FetchTaskRequest(ctx, req.TaskId)
	if err != nil {
		return nil, err
	}
	res := State(ctx).ACL.CheckTaskPerm(ctx, taskRequest, acls.PermTasksCancel)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	var outcome *tasks.CancelOpOutcome
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		outcome, err = srv.TasksManager.CancelTxn(ctx, &tasks.CancelOp{
			TaskRequest: taskRequest,
			KillRunning: req.KillRunning,
		})
		return err
	}, nil)

	if err != nil {
		logging.Errorf(ctx, "Error canceling task %s: %s", req.TaskId, err)
		return nil, status.Errorf(codes.Internal, "internal error canceling the task")
	}
	return &apipb.CancelResponse{
		Canceled:   outcome.Canceled,
		WasRunning: outcome.WasRunning,
	}, nil
}
