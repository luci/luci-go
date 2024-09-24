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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
)

const (
	// cancelTasksLimit is the maximum number of tasks to cancel in a single
	// request.
	cancelTasksLimit = 500
)

// CancelTasks implements bbpb.TaskBackendServer.
func (srv *TaskBackend) CancelTasks(ctx context.Context, req *bbpb.CancelTasksRequest) (*bbpb.CancelTasksResponse, error) {
	if err := srv.CheckBuildbucket(ctx); err != nil {
		return nil, err
	}

	l := len(req.GetTaskIds())
	switch {
	case l == 0:
		return &bbpb.CancelTasksResponse{}, nil
	case l > cancelTasksLimit:
		return nil, status.Errorf(codes.InvalidArgument, "requesting %d tasks when the allowed max is %d", l, cancelTasksLimit)
	}

	res := &bbpb.CancelTasksResponse{
		Responses: make([]*bbpb.CancelTasksResponse_Response, l),
	}

	if l == 1 {
		// Only one task to cancel, directly cancel it without sending to task queue.
		// Permission check is done in srv.TasksServer.CancelTask.
		_, err := srv.TasksServer.CancelTask(ctx, &apipb.TaskCancelRequest{
			TaskId:      req.TaskIds[0].Id,
			KillRunning: true,
		})
		if err != nil {
			if status.Code(err) == codes.Internal {
				// Return Internal error as an overall RPC error, to be consistent with
				// the batch variant.
				return nil, err
			}
			res.Responses[0] = &bbpb.CancelTasksResponse_Response{
				Response: &bbpb.CancelTasksResponse_Response_Error{
					Error: status.Convert(err).Proto(),
				},
			}
			return res, nil
		}
	}

	// Fetch TaskResultSummary and BuildTask entities and converts to bbpb.Task.
	buildTasks, err := srv.fetchTasks(ctx, req.TaskIds, acls.PermTasksCancel)
	if err != nil {
		return nil, err
	}

	activeTasks := make([]string, 0, l)
	for i, bt := range buildTasks {
		if bt.err != nil {
			res.Responses[i] = &bbpb.CancelTasksResponse_Response{
				Response: &bbpb.CancelTasksResponse_Response_Error{
					Error: status.Convert(bt.err).Proto(),
				},
			}
			continue
		}

		if bt.task == nil {
			panic("impossible")
		}

		res.Responses[i] = &bbpb.CancelTasksResponse_Response{
			Response: &bbpb.CancelTasksResponse_Response_Task{
				Task: bt.task,
			},
		}
		if bt.active {
			activeTasks = append(activeTasks, bt.task.Id.Id)
		}
	}

	if l == 1 {
		// We've canceled the single task.
		return res, nil
	}

	if len(activeTasks) == 0 {
		// Nothing to cancel, just return the current state of the tasks.
		return res, nil
	}

	// Finally enqueue a cloud task to cancel the active build tasks in batch asynchronously.
	if err = srv.TasksServer.TaskLifecycleTasks.EnqueueBatchCancel(ctx, activeTasks, true, "TaskBackend.CancelTasks", 0); err != nil {
		logging.Errorf(ctx, "Error enqueuing BatchCancelTask: %s", err)
		return nil, status.Errorf(codes.Internal, "failed to cancel tasks")
	}

	return res, nil
}
