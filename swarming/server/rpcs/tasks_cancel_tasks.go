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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cursor"
	"go.chromium.org/luci/swarming/server/cursor/cursorpb"
	"go.chromium.org/luci/swarming/server/model"
)

// CancelTasks implements the corresponding RPC method.
func (srv *TasksServer) CancelTasks(ctx context.Context, req *apipb.TasksCancelRequest) (*apipb.TasksCancelResponse, error) {
	var err error
	if req.Limit, err = ValidateLimit(req.Limit); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid limit: %s", err)
	}

	if len(req.Tags) == 0 {
		// Prevent accidental cancellation of everything.
		return nil, status.Errorf(codes.InvalidArgument, "tags are required when cancelling multiple tasks")
	}

	// Validate the rest of arguments and get TaskResultSummary datastore queries.
	state := apipb.StateQuery_QUERY_PENDING
	if req.KillRunning {
		state = apipb.StateQuery_QUERY_PENDING_RUNNING
	}
	queries, err := StartTaskListingRequest(ctx, &TaskListingRequest{
		Perm:       acls.PermPoolsCancelTask,
		Start:      req.Start,
		End:        req.End,
		State:      state,
		Tags:       req.Tags,
		Sort:       apipb.SortQuery_QUERY_CREATED_TS,
		Cursor:     req.Cursor,
		CursorKind: cursorpb.RequestKind_CANCEL_TASKS,
		Limit:      req.Limit,
		SplitMode:  srv.TaskQuerySplitMode,
	})
	switch {
	case err != nil:
		return nil, err
	case len(queries) == 0:
		return &apipb.TasksCancelResponse{Now: timestamppb.New(clock.Now(ctx))}, nil
	}

	out := &apipb.TasksCancelResponse{}
	var toCancel []string

	var dscursor *cursorpb.TasksCursor
	err = datastore.RunMulti(ctx, queries, func(task *model.TaskResultSummary) error {
		out.Matched++
		toCancel = append(toCancel, model.RequestKeyToTaskID(task.TaskRequestKey(), model.AsRequest))

		if out.Matched == req.Limit {
			dscursor = &cursorpb.TasksCursor{LastTaskRequestEntityId: task.TaskRequestKey().IntID()}
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		logging.Errorf(ctx, "Error querying TaskResultSummary: %s", err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching tasks")
	}

	if len(toCancel) > 0 {
		if err = srv.TaskLifecycleTasks.EnqueueBatchCancel(ctx, toCancel, req.KillRunning, "CancelTasks", 0); err != nil {
			logging.Errorf(ctx, "Error enqueuing BatchCancelTask: %s", err)
			return nil, status.Errorf(codes.Internal, "failed to cancel tasks")
		}
	}

	if dscursor != nil {
		out.Cursor, err = cursor.Encode(ctx, cursorpb.RequestKind_CANCEL_TASKS, dscursor)
		if err != nil {
			return nil, err
		}
	}

	out.Now = timestamppb.New(clock.Now(ctx))
	return out, nil
}
