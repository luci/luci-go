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

package rpc

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func validateStartBuildRequest(ctx context.Context, req *pb.StartBuildRequest) error {
	if procRes := protowalk.Fields(req, &protowalk.RequiredProcessor{}); procRes != nil {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			logging.Infof(ctx, strings.Join(resStrs, ". "))
		}
		return procRes.Err()
	}
	return nil
}

func startBuildOnBackendOnFirstReq(ctx context.Context, req *pb.StartBuildRequest, b *model.Build, infra *model.BuildInfra) error {
	taskID := infra.Proto.Backend.Task.GetId()

	if taskID.GetId() != "" && taskID.GetId() != req.TaskId {
		// The build has been associated with another task, possible from a previous
		// RegisterBuildTask call from a different task.
		return buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has associated with task %q", req.BuildId, taskID.Id))
	}

	// Start the build.
	b.StartBuildRequestID = req.RequestId
	updateBuildToken, err := buildtoken.GenerateToken(ctx, b.ID, pb.TokenBody_BUILD)
	if err != nil {
		return errors.Annotate(err, "failed to generate BUILD token for build %d", b.ID).Err()
	}
	b.UpdateToken = updateBuildToken
	protoutil.SetStatus(clock.Now(ctx), b.Proto, pb.Status_STARTED)
	toSave := []any{b}

	if taskID.GetId() == "" {
		// First handshake, associate the task with the build.
		taskID.Id = req.TaskId
		toSave = append(toSave, infra)
	}
	err = datastore.Put(ctx, toSave)
	if err != nil {
		return errors.Annotate(err, "failed to start build %d: %s", b.ID, err).Err()
	}

	// Notify Pubsub.
	// NotifyPubSub is a Transactional task, so do it inside the transaction.
	_ = tasks.NotifyPubSub(ctx, b)

	return nil
}

func startBuildOnBackend(ctx context.Context, req *pb.StartBuildRequest) (*model.Build, bool, error) {
	var b *model.Build
	buildStatusChanged := false
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var infra *model.BuildInfra
		var err error
		b, infra, err = getBuildAndInfra(ctx, req.BuildId)
		if err != nil {
			if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
				return err
			}
			return errors.Annotate(err, "failed to get build %d", req.BuildId).Err()
		}

		if infra.Proto.GetBackend().GetTask() == nil {
			return errors.Reason("the build %d does not run on task backend", req.BuildId).Err()
		}

		if b.StartBuildRequestID == "" {
			// First StartBuild for the build.
			err = startBuildOnBackendOnFirstReq(ctx, req, b, infra)
			if err == nil {
				buildStatusChanged = true
			}
			return err
		}

		return checkSubsequentRequest(req, b.StartBuildRequestID, infra.Proto.Backend.Task.GetId().GetId())
	}, nil)
	if txErr != nil {
		return nil, false, txErr
	}
	return b, buildStatusChanged, nil
}

func checkSubsequentRequest(req *pb.StartBuildRequest, savedReqID, savedTaskID string) error {
	// Subsequent StartBuild request.
	if savedReqID != req.RequestId {
		// Different request id, deduplicate.
		return buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has recorded another StartBuild with request id %q", req.BuildId, savedReqID))
	}

	if savedTaskID != req.TaskId {
		// Same request id, different task id.
		return errors.Reason("build %d has associated with task id %q with StartBuild request id %q", req.BuildId, savedTaskID, savedReqID).Tag(buildbucket.TaskWithCollidedRequestID).Err()
	}

	// Idempotent
	return nil
}

// StartBuild handles a request to start a build. Implements pb.BuildsServer.
func (*Builds) StartBuild(ctx context.Context, req *pb.StartBuildRequest) (*pb.StartBuildResponse, error) {
	if err := validateStartBuildRequest(ctx, req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	var b *model.Build
	var buildStatusChanged bool
	var err error
	// TODO(crbug.com/1416971): support build on swarming.
	_, _, err = validateToken(ctx, req.BuildId, pb.TokenBody_START_BUILD)
	switch {
	case err != nil:
		return nil, appstatus.BadRequest(errors.Annotate(err, "invalid start build token for starting build %d", req.BuildId).Err())
	default:
		b, buildStatusChanged, err = startBuildOnBackend(ctx, req)
	}
	if err != nil {
		return nil, err
	}

	if buildStatusChanged {
		// Update metrics.
		logging.Infof(ctx, "Build %d: started", b.ID)
		metrics.BuildStarted(ctx, b)
	}

	mask, err := model.NewBuildMask("", nil, &pb.BuildMask{AllFields: true})
	if err != nil {
		return nil, errors.Annotate(err, "failed to construct build mask").Err()
	}
	bp, err := b.ToProto(ctx, mask, nil)
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate build proto from model").Err()
	}

	return &pb.StartBuildResponse{Build: bp, UpdateBuildToken: b.UpdateToken}, nil
}
