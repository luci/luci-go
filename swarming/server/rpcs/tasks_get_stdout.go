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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// MaxOutputLength is the maximum (and default) amount of output to fetch,
// mostly for compatibility with previous behavior. See model.ChunkSize.
const MaxOutputLength = 160 * model.ChunkSize

// GetStdout implements the corresponding RPC method.
func (*TasksServer) GetStdout(ctx context.Context, req *apipb.TaskIdWithOffsetRequest) (*apipb.TaskOutputResponse, error) {
	if req.TaskId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "task_id is required")
	}
	if req.Offset < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "offset must be non-negative")
	}
	if req.Length < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "length must be non-negative")
	}

	trKey, err := model.TaskIDToRequestKey(ctx, req.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "task_id %s: %s", req.TaskId, err)
	}
	trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, trKey)}
	err = datastore.Get(ctx, trs)
	switch {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no such task")
	case err != nil:
		logging.Errorf(ctx, "Error fetching TaskResultSummary %s: %s", req.TaskId, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching the task")
	}

	res := State(ctx).ACL.CheckTaskPerm(ctx, trs, acls.PermTasksGet)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	length := req.Length
	if length == 0 || length > MaxOutputLength {
		length = MaxOutputLength
	}
	output, err := trs.GetOutput(ctx, req.Offset, length)
	if err != nil {
		logging.Errorf(ctx, "Error getting task result output: %s", err)
		return nil, status.Errorf(codes.Internal, "error getting task result output")
	}

	return &apipb.TaskOutputResponse{
		Output: output,
		State:  trs.State,
	}, nil
}
