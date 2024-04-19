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

package bbtaskbackend

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/util/taskbackendutil"
)

const (
	// fetchTasksLimit is the maximum number of tasks to fetch in a single FetchTasks request.
	fetchTasksLimit = 1000
)

// FetchTasks implements bbpb.TaskBackendServer.
func (t *TaskBackend) FetchTasks(ctx context.Context, req *bbpb.FetchTasksRequest) (*bbpb.FetchTasksResponse, error) {
	res := &bbpb.FetchTasksResponse{}
	l := len(req.GetTaskIds())
	if l == 0 {
		return res, nil
	} else if l > fetchTasksLimit {
		return nil, status.Errorf(codes.InvalidArgument, "Requesting %d tasks when the allowed max is %d.", l, fetchTasksLimit)
	}

	idxMap := make([]int, 0, l) // a map of current index to the original index in req.
	res.Responses = make([]*bbpb.FetchTasksResponse_Response, l)
	resultSummaries := make([]*model.TaskResultSummary, 0, l)
	buildTasks := make([]*model.BuildTask, 0, l)
	for i, taskID := range req.GetTaskIds() {
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID.Id)
		if err != nil {
			res.Responses[i] = &bbpb.FetchTasksResponse_Response{
				Response: &bbpb.FetchTasksResponse_Response_Error{
					Error: status.New(codes.InvalidArgument, err.Error()).Proto(),
				},
			}
			continue
		}
		buildTasks = append(buildTasks, &model.BuildTask{Key: model.BuildTaskKey(ctx, reqKey)})
		resultSummaries = append(resultSummaries, &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)})
		idxMap = append(idxMap, i)
	}

	// Fetch TaskResultSummary and BuildTask entities
	var btErrs, rsErrs errors.MultiError
	if err := datastore.Get(ctx, resultSummaries, buildTasks); err != nil {
		me, ok := err.(errors.MultiError)
		if !ok {
			// Top Level error
			logging.Errorf(ctx, "failed to fetch task result summaries or build tasks: %s", err)
			return nil, status.Errorf(codes.Internal, "failed to fetch task result summaries or build tasks")
		}
		if me[0] != nil {
			rsErrs = me[0].(errors.MultiError) // errors for TaskResultSummary entities
		}
		if me[1] != nil {
			btErrs = me[1].(errors.MultiError) // errors for BuildTask entities
		}
	}

	// Convert to Buildbucket FetchTasks response
	checker := acls.NewChecker(ctx, t.cfgProvider.Config(ctx))
	for i := 0; i < len(resultSummaries); i++ {
		var errStatus *status.Status
		tID := req.TaskIds[idxMap[i]].Id

		// Check errors
		switch {
		case btErrs != nil && btErrs[i] != nil:
			if errors.Is(btErrs[i], datastore.ErrNoSuchEntity) {
				errStatus = status.Newf(codes.NotFound, "BuildTask not found %s", tID)
			} else {
				errStatus = status.Newf(codes.Internal, "failed to fetch BuildTask %s", tID)
				logging.Errorf(ctx, "failed to fetch BuildTask %s: %s", tID, btErrs[i])
			}
		case rsErrs != nil && rsErrs[i] != nil:
			if errors.Is(rsErrs[i], datastore.ErrNoSuchEntity) {
				errStatus = status.Newf(codes.NotFound, "TaskResultSummary not found %s", tID)
			} else {
				errStatus = status.Newf(codes.Internal, "failed to fetch TaskResultSummary %s", tID)
				logging.Errorf(ctx, "failed to fetch TaskResultSummary %s: %s", tID, rsErrs[i])
			}
		}
		if errStatus != nil {
			res.Responses[idxMap[i]] = &bbpb.FetchTasksResponse_Response{
				Response: &bbpb.FetchTasksResponse_Response_Error{
					Error: errStatus.Proto(),
				},
			}
			continue
		}

		// Check permissions
		rst := checker.CheckTaskPerm(ctx, resultSummaries[i].TaskAuthInfo(), acls.PermTasksGet)
		if !rst.Permitted {
			s := status.Convert(rst.ToGrpcErr())
			res.Responses[idxMap[i]] = &bbpb.FetchTasksResponse_Response{
				Response: &bbpb.FetchTasksResponse_Response_Error{
					Error: s.Proto(),
				},
			}
			continue
		}

		// Convert
		res.Responses[idxMap[i]] = toFetchTasksResponse(resultSummaries[i], buildTasks[i], tID, t.bbTarget)
	}

	return res, nil
}

// toFetchTasksResponse convert the given task result and build task to a FetchTasksResponse.
func toFetchTasksResponse(result *model.TaskResultSummary, bTask *model.BuildTask, tID, target string) *bbpb.FetchTasksResponse_Response {
	// try to get bot_dimensions from build_task first, if not get it from result.
	botDims := bTask.BotDimensions
	if len(botDims) == 0 {
		botDims = result.BotDimensions
	}
	bbTask := &bbpb.Task{
		Id: &bbpb.TaskID{
			Id:     model.RequestKeyToTaskID(result.TaskRequestKey(), model.AsRequest),
			Target: target,
		},
		UpdateId: bTask.UpdateID,
		Details: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"bot_dimensions": {
					Kind: &structpb.Value_StructValue{
						StructValue: botDims.ToStructPB(),
					},
				},
			},
		},
	}
	taskbackendutil.SetBBStatus(result.State, result.Failure, bbTask)
	return &bbpb.FetchTasksResponse_Response{
		Response: &bbpb.FetchTasksResponse_Response_Task{
			Task: bbTask,
		},
	}
}
