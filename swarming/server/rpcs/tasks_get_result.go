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

package rpcs

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// GetResult implements the corresponding RPC method.
func (*TasksServer) GetResult(ctx context.Context, req *apipb.TaskIdWithPerfRequest) (*apipb.TaskResultResponse, error) {
	if req.TaskId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "task_id is required")
	}

	key, err := model.TaskIDToRequestKey(ctx, req.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "task_id %s: %s", req.TaskId, err)
	}

	trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, key)}
	switch err = datastore.Get(ctx, trs); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no such task")
	case err != nil:
		logging.Errorf(ctx, "Error fetching TaskResultSummary %s: %s", req.TaskId, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching the task")
	}

	res := State(ctx).ACL.CheckTaskPerm(ctx, trs.TaskAuthInfo(), acls.PermTasksGet)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	resp := trs.ToProto()

	if req.IncludePerformanceStats {
		if key := trs.PerformanceStatsKey(ctx); key != nil {
			ps := &model.PerformanceStats{Key: key}
			err = datastore.Get(ctx, ps)
			if err == nil {
				resp.PerformanceStats, err = ps.ToProto()
			}
			switch {
			case errors.Is(err, datastore.ErrNoSuchEntity):
				// Likely the task died before it could reports its performance stats.
				// This is fine. Just don't attach performance stats to the response.
				logging.Warningf(ctx, "No performance stats for %q (task state is %s)", req.TaskId, resp.State)
			case err != nil:
				logging.Errorf(ctx, "Error fetching PerformanceStats %q: %s", req.TaskId, err)
				return nil, status.Errorf(codes.Internal, "datastore error fetching performance stats")
			}
		}
	}

	return resp, nil
}
