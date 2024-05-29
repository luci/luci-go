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

// ListTasks implements the corresponding RPC method.
func (srv *TasksServer) ListTasks(ctx context.Context, req *apipb.TasksWithPerfRequest) (*apipb.TaskListResponse, error) {
	var err error
	if req.Limit, err = ValidateLimit(req.Limit); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid limit: %s", err)
	}

	// Validate the rest of arguments and get TaskResultSummary datastore queries.
	queries, err := StartTaskListingRequest(ctx, &TaskListingRequest{
		Perm:       acls.PermPoolsListTasks,
		Start:      req.Start,
		End:        req.End,
		State:      req.State,
		Sort:       req.Sort,
		Tags:       req.Tags,
		Cursor:     req.Cursor,
		CursorKind: cursorpb.RequestKind_LIST_TASKS,
		Limit:      req.Limit,
		SplitMode:  srv.TaskQuerySplitMode,
	})
	switch {
	case err != nil:
		return nil, err
	case len(queries) == 0:
		return &apipb.TaskListResponse{Now: timestamppb.New(clock.Now(ctx))}, nil
	}

	var stats *model.PerformanceStatsFetcher
	if req.IncludePerformanceStats {
		stats = model.NewPerformanceStatsFetcher(ctx)
		defer stats.Close()
	}

	out := &apipb.TaskListResponse{}

	var dscursor *cursorpb.TasksCursor
	err = datastore.RunMulti(ctx, queries, func(task *model.TaskResultSummary) error {
		taskpb := task.ToProto()
		if stats != nil {
			stats.Fetch(ctx, taskpb, task)
		}
		out.Items = append(out.Items, taskpb)
		if len(out.Items) == int(req.Limit) {
			dscursor = &cursorpb.TasksCursor{LastTaskRequestEntityId: task.TaskRequestKey().IntID()}
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		logging.Errorf(ctx, "Error querying TaskResultSummary: %s", err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching tasks")
	}
	if dscursor != nil {
		out.Cursor, err = cursor.Encode(ctx, cursorpb.RequestKind_LIST_TASKS, dscursor)
		if err != nil {
			return nil, err
		}
	}

	if stats != nil {
		if err := stats.Finish(out.Items); err != nil {
			logging.Errorf(ctx, "Error fetching PerformanceStats: %s", err)
			return nil, status.Errorf(codes.Internal, "error fetching performance stats")
		}
	}

	out.Now = timestamppb.New(clock.Now(ctx))
	return out, nil
}
