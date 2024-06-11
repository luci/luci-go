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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

const (
	// fetchTasksLimit is the maximum number of tasks to fetch in a single
	// FetchTasks request.
	fetchTasksLimit = 1000
)

// FetchTasks implements bbpb.TaskBackendServer.
func (srv *TaskBackend) FetchTasks(ctx context.Context, req *bbpb.FetchTasksRequest) (*bbpb.FetchTasksResponse, error) {
	if err := srv.CheckBuildbucket(ctx); err != nil {
		return nil, err
	}

	l := len(req.GetTaskIds())
	switch {
	case l == 0:
		return &bbpb.FetchTasksResponse{}, nil
	case l > fetchTasksLimit:
		return nil, status.Errorf(codes.InvalidArgument, "requesting %d tasks when the allowed max is %d", l, fetchTasksLimit)
	}

	res := &bbpb.FetchTasksResponse{
		Responses: make([]*bbpb.FetchTasksResponse_Response, l),
	}

	idxMap := make([]int, 0, l) // a map of current index to the original index in req.
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

	// Fetch TaskResultSummary and BuildTask entities.
	var btErrs, rsErrs errors.MultiError
	if err := datastore.Get(ctx, resultSummaries, buildTasks); err != nil {
		me, ok := err.(errors.MultiError)
		if !ok {
			logging.Errorf(ctx, "Failed to fetch task result summaries or build tasks: %s", err)
			return nil, status.Errorf(codes.Internal, "failed to fetch task result summaries or build tasks")
		}
		if me[0] != nil {
			rsErrs = me[0].(errors.MultiError) // errors for TaskResultSummary entities
		}
		if me[1] != nil {
			btErrs = me[1].(errors.MultiError) // errors for BuildTask entities
		}
	}

	// Convert to FetchTasksResponse.
	checker := State(ctx).ACL
	for i := 0; i < len(resultSummaries); i++ {
		tID := req.TaskIds[idxMap[i]].Id

		// Check errors.
		var errStatus *status.Status
		switch {
		case rsErrs != nil && rsErrs[i] != nil:
			if errors.Is(rsErrs[i], datastore.ErrNoSuchEntity) {
				errStatus = status.Newf(codes.NotFound, "%s: no such task", tID)
			} else {
				errStatus = status.Newf(codes.Internal, "%s: failed to fetch TaskResultSummary", tID)
				logging.Errorf(ctx, "Failed to fetch TaskResultSummary %s: %s", tID, rsErrs[i])
			}
		case btErrs != nil && btErrs[i] != nil:
			if errors.Is(btErrs[i], datastore.ErrNoSuchEntity) {
				errStatus = status.Newf(codes.NotFound, "%s: not a Buildbucket task", tID)
			} else {
				errStatus = status.Newf(codes.Internal, "%s: failed to fetch BuildTask", tID)
				logging.Errorf(ctx, "Failed to fetch BuildTask %s: %s", tID, btErrs[i])
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

		// Check permissions.
		rst := checker.CheckTaskPerm(ctx, resultSummaries[i], acls.PermTasksGet)
		if !rst.Permitted {
			s := status.Convert(rst.ToGrpcErr())
			res.Responses[idxMap[i]] = &bbpb.FetchTasksResponse_Response{
				Response: &bbpb.FetchTasksResponse_Response_Error{
					Error: s.Proto(),
				},
			}
			continue
		}

		// Convert.
		res.Responses[idxMap[i]] = &bbpb.FetchTasksResponse_Response{
			Response: &bbpb.FetchTasksResponse_Response_Task{
				Task: buildTasks[i].ToProto(resultSummaries[i], srv.BuildbucketTarget),
			},
		}
	}

	return res, nil
}
