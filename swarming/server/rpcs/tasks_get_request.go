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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// taskRequestToResponse converts a model.TaskRequest to apipb.TaskRequestResponse.
func taskRequestToResponse(ctx context.Context, req *apipb.TaskIdRequest, taskRequest *model.TaskRequest) (*apipb.TaskRequestResponse, error) {
	// Create the list of task slices.
	taskSlices := make([]*apipb.TaskSlice, len(taskRequest.TaskSlices))
	for i, slice := range taskRequest.TaskSlices {
		taskSlices[i] = slice.ToProto()
	}
	// Ensure that there is a task slice to pull TaskProperties from.
	properties := &apipb.TaskProperties{}
	if len(taskSlices) != 0 {
		properties = taskSlices[0].Properties
	}
	return &apipb.TaskRequestResponse{
		TaskId:               req.TaskId,
		ExpirationSecs:       int32(taskRequest.Expiration.Second()),
		Name:                 taskRequest.Name,
		ParentTaskId:         taskRequest.ParentTaskID.Get(),
		Priority:             int32(taskRequest.Priority),
		Properties:           properties,
		Tags:                 taskRequest.Tags,
		CreatedTs:            timestamppb.New(taskRequest.Created),
		User:                 taskRequest.User,
		Authenticated:        string(taskRequest.Authenticated),
		TaskSlices:           taskSlices,
		ServiceAccount:       taskRequest.ServiceAccount,
		Realm:                taskRequest.Realm,
		PubsubTopic:          taskRequest.PubSubTopic,
		PubsubUserdata:       taskRequest.PubSubUserData,
		BotPingToleranceSecs: int32(taskRequest.BotPingToleranceSecs),
		RbeInstance:          taskRequest.RBEInstance,
		Resultdb:             taskRequest.ResultDB.ToProto(),
	}, nil
}

// GetRequest fetches a model.TaskRequest for a given apipb.TaskIdRequest.
func (*TasksServer) GetRequest(ctx context.Context, req *apipb.TaskIdRequest) (*apipb.TaskRequestResponse, error) {
	if req.TaskId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "task_id is required")
	}
	taskReqKey, err := model.TaskIDToRequestKey(ctx, req.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "task_id %s: %s", req.TaskId, err)
	}
	taskRequest := &model.TaskRequest{Key: taskReqKey}
	err = datastore.Get(ctx, taskRequest)
	switch {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no such task")
	case err != nil:
		logging.Errorf(ctx, "Error fetching TaskRequest %s: %s", req.TaskId, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching the task")
	}
	err = State(ctx).ACL.CheckTaskPerm(ctx, taskRequest.TaskAuthInfo(), acls.PermTasksGet)
	if err != nil {
		// The error from CheckTaskPerm must be returned as is. See CheckTaskPerm doc.
		return nil, err
	}
	return taskRequestToResponse(ctx, req, taskRequest)
}
