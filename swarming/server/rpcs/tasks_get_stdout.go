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

const (
	// Default byte offset to start fetching.
	DefaultOffset = 0

	// Maximum content fetched at once, mostly for compatibility with previous
	// behavior. See TaskResult.ChunkSize.
	MaxOutputLength = 16 * 1000 * 1024
)

// GetStdout implements the GetStdout RPC.
func (*TasksServer) GetStdout(ctx context.Context, req *apipb.TaskIdWithOffsetRequest) (*apipb.TaskOutputResponse, error) {
	if req.TaskId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "task_id is required")
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
	res := State(ctx).ACL.CheckTaskPerm(ctx, trs.TaskAuthInfo(), acls.PermTasksGet)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}
	length := req.GetLength()
	if length <= 0 || length > MaxOutputLength {
		length = MaxOutputLength
	}
	offset := req.GetOffset()
	if offset < 0 {
		offset = DefaultOffset
	}
	output, err := trs.GetOutput(ctx, length, offset)
	if err != nil {
		logging.Errorf(ctx, "Error getting task result output: %s", err.Error())
		return nil, status.Errorf(codes.Internal, "error getting task result output")
	}
	return &apipb.TaskOutputResponse{
		Output: output,
		State:  trs.State,
	}, nil
}
