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
	"time"

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

// ListBotTasks implements the corresponding RPC method.
func (s *BotsServer) ListBotTasks(ctx context.Context, req *apipb.BotTasksRequest) (*apipb.TaskListResponse, error) {
	if req.BotId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bot_id is required")
	}

	var err error
	if req.Limit, err = ValidateLimit(req.Limit); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid limit: %s", err)
	}

	// Some state filters make no sense for this RPC, since applying them will
	// always result in empty response. For example, pending tasks aren't
	// associated with any bots yet, and thus ListBotTasks will never return them.
	// Let the caller know.
	if err := validateBotTasksQueryStateFilter(req.State); err != nil {
		return nil, err
	}

	// Fail cleanly when using a combination of parameters for which we have no
	// datastore indexes (see TaskRunResult in index.yaml). The corresponding
	// indexes can be added if necessary (but they have non-trivial cost).
	if err := validateBotTasksQueryIndexed(req.Sort, req.State); err != nil {
		return nil, err
	}

	var dscursor datastore.Cursor
	if req.Cursor != "" {
		var err error
		dscursor, err = cursor.DecodeOpaqueCursor(ctx, cursorpb.RequestKind_LIST_BOT_TASKS, req.Cursor)
		if err != nil {
			return nil, err
		}
	}

	res := State(ctx).ACL.CheckBotPerm(ctx, req.BotId, acls.PermPoolsListTasks)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	q := model.TaskRunResultQuery().Eq("bot_id", req.BotId)
	if req.State != apipb.StateQuery_QUERY_ALL {
		q = model.FilterTasksByState(q, req.State)
	}
	q = q.Limit(req.Limit)
	if dscursor != nil {
		q = q.Start(dscursor)
	}

	// Apply ordering and the time range filter. Filtering by creation time is
	// special, since entity keys are already ordered by creation time (and thus
	// this ordering doesn't require any extra indices, but requires special
	// filtering).
	var startTS, endTS time.Time
	if req.Start != nil {
		startTS = req.Start.AsTime()
	}
	if req.End != nil {
		endTS = req.End.AsTime()
	}
	switch req.Sort {
	case apipb.SortQuery_QUERY_CREATED_TS:
		// Note that this sorts based on when the task was submitted to the server,
		// not by when it was assigned to this bot. The timestamp when a task is
		// assigned to the bot is stored in "started_ts" field and it used with
		// QUERY_STARTED_TS sort order.
		var err error
		q, err = model.FilterTasksByCreationTime(ctx, q, startTS, endTS)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid time range: %s", err)
		}
	case apipb.SortQuery_QUERY_COMPLETED_TS:
		q = model.FilterTasksByTimestampField(q, "completed_ts", startTS, endTS)
	case apipb.SortQuery_QUERY_STARTED_TS:
		q = model.FilterTasksByTimestampField(q, "started_ts", startTS, endTS)
	default:
		panic("impossible, already checked")
	}

	// TODO: Handle req.IncludePerformanceStats.

	out := &apipb.TaskListResponse{}

	dscursor = nil
	err = datastore.Run(ctx, q, func(task *model.TaskRunResult, cb datastore.CursorCB) error {
		out.Items = append(out.Items, task.ToProto())
		if len(out.Items) == int(req.Limit) {
			var err error
			if dscursor, err = cb(); err != nil {
				return err
			}
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		logging.Errorf(ctx, "Error querying TaskRunResult for %q: %s", req.BotId, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching tasks")
	}

	if dscursor != nil {
		out.Cursor, err = cursor.EncodeOpaqueCursor(ctx, cursorpb.RequestKind_LIST_BOT_TASKS, dscursor)
		if err != nil {
			return nil, err
		}
	}

	out.Now = timestamppb.New(clock.Now(ctx))
	return out, nil
}

// validateBotTasksQueryStateFilter checks if the state filter makes sense.
func validateBotTasksQueryStateFilter(state apipb.StateQuery) error {
	switch state {
	case apipb.StateQuery_QUERY_PENDING:
		return status.Errorf(codes.InvalidArgument,
			"default QUERY_PENDING state filter will always result in empty response, "+
				"since pending tasks are not associated with bots: use a different filter (perhaps QUERY_ALL?)")
	case apipb.StateQuery_QUERY_PENDING_RUNNING:
		return status.Errorf(codes.InvalidArgument,
			"QUERY_PENDING_RUNNING state filter is equivalent to QUERY_RUNNING, "+
				"since pending tasks are not associated with bots: use QUERY_RUNNING filter explicitly")
	case apipb.StateQuery_QUERY_EXPIRED:
		return status.Errorf(codes.InvalidArgument,
			"QUERY_EXPIRED state filter will always result in empty response, "+
				"since expired tasks are not associated with bots")
	case apipb.StateQuery_QUERY_DEDUPED:
		return status.Errorf(codes.InvalidArgument,
			"QUERY_DEDUPED state filter will always result in empty response, "+
				"since dedupped tasks are not associated with bots")
	case apipb.StateQuery_QUERY_NO_RESOURCE:
		return status.Errorf(codes.InvalidArgument,
			"QUERY_NO_RESOURCE state filter will always result in empty response, "+
				"since NO_RESOURCE tasks are not associated with bots")
	case apipb.StateQuery_QUERY_CANCELED:
		return status.Errorf(codes.InvalidArgument,
			"QUERY_CANCELED state filter will always result in empty response, "+
				"since canceled tasks are not associated with bots: use a different filter (perhaps QUERY_KILLED?)")
	default:
		return nil
	}
}

// validateBotTasksQueryIndexed checks we have indexes required by the query.
func validateBotTasksQueryIndexed(sort apipb.SortQuery, state apipb.StateQuery) error {
	switch sort {
	case apipb.SortQuery_QUERY_CREATED_TS:
		// No limits for this default sort order.
		return nil
	case apipb.SortQuery_QUERY_COMPLETED_TS, apipb.SortQuery_QUERY_STARTED_TS:
		if state != apipb.StateQuery_QUERY_ALL {
			return status.Errorf(codes.InvalidArgument, "a state filter other than QUERY_ALL when using non-default sort order is not supported currently, open a bug if needed")
		}
		return nil
	case apipb.SortQuery_QUERY_ABANDONED_TS:
		return status.Errorf(codes.InvalidArgument, "sorting by abandoned time is not supported currently, open a bug if needed")
	default:
		return status.Errorf(codes.InvalidArgument, "invalid sort order")
	}
}
