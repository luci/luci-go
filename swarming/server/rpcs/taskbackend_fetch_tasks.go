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
	"go.chromium.org/luci/server/auth/realms"

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

	buildTasks, err := srv.fetchTasks(ctx, req.TaskIds, acls.PermTasksGet)
	if err != nil {
		return nil, err
	}
	for i, bt := range buildTasks {
		if bt.err != nil {
			s := status.Convert(bt.err)
			res.Responses[i] = &bbpb.FetchTasksResponse_Response{
				Response: &bbpb.FetchTasksResponse_Response_Error{
					Error: s.Proto(),
				},
			}
			continue
		}

		if bt.task == nil {
			panic("impossible")
		}

		res.Responses[i] = &bbpb.FetchTasksResponse_Response{
			Response: &bbpb.FetchTasksResponse_Response_Task{
				Task: bt.task,
			},
		}
	}

	return res, nil
}

type buildTask struct {
	err    error      // non-nil if failed to fetch
	task   *bbpb.Task // set only if err is nil
	active bool
}

// fetchTasks fetches TaskResultSummary and BuildTask entities and converts
// them to a list of bbpb.Task.
// If the call fails in some general way unrelated to tasks being fetched,
// returns nil results and an error. Otherwise returns a list of buildTask
// guaranteed to have the same length as taskIDs. Error related to a
// particular task are represented by buildTask.err. All errors are gRPC
// errors.
func (srv *TaskBackend) fetchTasks(ctx context.Context, taskIDs []*bbpb.TaskID, perm realms.Permission) ([]*buildTask, error) {
	l := len(taskIDs)
	tasks := make([]*buildTask, l)
	idxMap := make([]int, 0, l) // a map of current index to the original index in req.
	reqKeys := make([]*datastore.Key, 0, l)
	for i, taskID := range taskIDs {
		if taskID.GetTarget() != srv.BuildbucketTarget {
			tasks[i] = &buildTask{
				err: status.Errorf(codes.InvalidArgument,
					"wrong buildbucket target %s, while expecting %s",
					taskID.GetTarget(), srv.BuildbucketTarget),
			}
			continue
		}
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID.Id)
		if err != nil {
			tasks[i] = &buildTask{err: status.Error(codes.InvalidArgument, err.Error())}
			continue
		}
		reqKeys = append(reqKeys, reqKey)
		idxMap = append(idxMap, i)
	}

	// Fetch TaskResultSummary and BuildTask entities.
	resultSummaries, buildTasks, err := fetchBuildTaskEntities(ctx, reqKeys)
	var entMerr errors.MultiError
	if err != nil {
		var ok bool
		entMerr, ok = err.(errors.MultiError)
		if !ok {
			return nil, err
		}
	}

	// Convert to FetchTasksResponse.
	checker := State(ctx).ACL
	for i := range resultSummaries {
		// Check errors.
		if entMerr != nil && entMerr[i] != nil {
			tasks[idxMap[i]] = &buildTask{err: entMerr[i]}
			continue
		}

		// Check permissions.
		rst := checker.CheckTaskPerm(ctx, resultSummaries[i], perm)
		if !rst.Permitted {
			tasks[idxMap[i]] = &buildTask{err: rst.ToGrpcErr()}
			continue
		}

		// Convert.
		tasks[idxMap[i]] = &buildTask{
			task:   buildTasks[i].ToProto(resultSummaries[i], srv.BuildbucketTarget),
			active: resultSummaries[i].IsActive(),
		}
	}

	return tasks, nil
}

func fetchBuildTaskEntities(ctx context.Context, reqKeys []*datastore.Key) ([]*model.TaskResultSummary, []*model.BuildTask, error) {
	l := len(reqKeys)
	resultSummaries := make([]*model.TaskResultSummary, 0, l)
	buildTasks := make([]*model.BuildTask, 0, l)
	merr := make(errors.MultiError, l)
	for _, reqKey := range reqKeys {
		buildTasks = append(buildTasks, &model.BuildTask{
			Key: model.BuildTaskKey(ctx, reqKey)})
		resultSummaries = append(resultSummaries, &model.TaskResultSummary{
			Key: model.TaskResultSummaryKey(ctx, reqKey)})
	}

	var btErrs, rsErrs errors.MultiError
	if err := datastore.Get(ctx, resultSummaries, buildTasks); err != nil {
		me, ok := err.(errors.MultiError)
		if !ok {
			logging.Errorf(ctx, "Failed to fetch task result summaries or build tasks: %s", err)
			return nil, nil, status.Errorf(codes.Internal, "failed to fetch task result summaries or build tasks")
		}
		if me[0] != nil {
			rsErrs = me[0].(errors.MultiError) // errors for TaskResultSummary entities
		}
		if me[1] != nil {
			btErrs = me[1].(errors.MultiError) // errors for BuildTask entities
		}
	}

	for i := range reqKeys {
		tID := model.RequestKeyToTaskID(reqKeys[i], model.AsRequest)
		switch {
		case rsErrs != nil && rsErrs[i] != nil:
			if errors.Is(rsErrs[i], datastore.ErrNoSuchEntity) {
				merr[i] = status.Errorf(codes.NotFound, "%s: no such task", tID)
			} else {
				logging.Errorf(ctx, "Failed to fetch TaskResultSummary %s: %s", tID, rsErrs[i])
				merr[i] = status.Errorf(codes.Internal, "%s: failed to fetch TaskResultSummary", tID)
			}
		case btErrs != nil && btErrs[i] != nil:
			if errors.Is(btErrs[i], datastore.ErrNoSuchEntity) {
				merr[i] = status.Errorf(codes.NotFound, "%s: not a Buildbucket task", tID)
			} else {
				logging.Errorf(ctx, "Failed to fetch BuildTask %s: %s", tID, btErrs[i])
				merr[i] = status.Errorf(codes.Internal, "%s: failed to fetch BuildTask", tID)
			}
		}
	}
	return resultSummaries, buildTasks, merr
}
