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
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildstatus"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var sbrWalker = protowalk.NewWalker[*pb.StartBuildRequest](protowalk.RequiredProcessor{})

func validateStartBuildRequest(ctx context.Context, req *pb.StartBuildRequest) error {
	if procRes := sbrWalker.Execute(req); !procRes.Empty() {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			logging.Infof(ctx, strings.Join(resStrs, ". "))
		}
		return procRes.Err()
	}
	return nil
}

// startBuildOnSwarming starts a build if it runs on swarming.
//
// For builds on Swarming, the swarming task id and update_token have been
// saved in datastore during task creation.
func startBuildOnSwarming(ctx context.Context, req *pb.StartBuildRequest, tok string) (*model.Build, bool, error) {
	var b *model.Build
	buildStatusChanged := false
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		entities, err := common.GetBuildEntities(ctx, req.BuildId, model.BuildKind, model.BuildInfraKind)
		if err != nil {
			return errors.Annotate(err, "failed to get build %d", req.BuildId).Err()
		}
		b = entities[0].(*model.Build)
		infra := entities[1].(*model.BuildInfra)

		if infra.Proto.GetSwarming() == nil {
			return appstatus.Errorf(codes.Internal, "the build %d does not run on swarming", req.BuildId)
		}

		// First StartBuild request.
		if b.StartBuildRequestID == "" {
			if infra.Proto.Swarming.TaskId != req.TaskId && infra.Proto.Swarming.TaskId != "" {
				// Duplicated task.
				return buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has associated with task %q", req.BuildId, infra.Proto.Swarming.TaskId))
			}

			if protoutil.IsEnded(b.Status) {
				// The build has ended.
				// For example the StartBuild request reaches Buildbucket late, when the task
				// has crashed (e.g. BOT_DIED).
				return appstatus.Errorf(codes.FailedPrecondition, "cannot start ended build %d", b.ID)
			}

			var toPut []any
			if infra.Proto.Swarming.TaskId == "" {
				// In rare cases that a StartBuild request for a build can reach Buildbucket
				// before tasks.CreateSwarmingBuildTask: e.g.
				// 1. CreateSwarmingBuildTask for a build succeeds to create a
				//    swarming task for the build, but it fails to save the task to datastore,
				//    then this task needs to retry
				// 2. in the meantime swarming starts to run the task created in 1, sends
				//    a StartBuild request
				// 3. after StartBuild, the retried CreateSwarmingBuildTask tries to
				//    update datastore again with the same task id (because now creating task
				//    is idempotent).
				infra.Proto.Swarming.TaskId = req.TaskId
				b.UpdateToken = tok
				toPut = append(toPut, infra)
			}

			// Start the build.
			b.StartBuildRequestID = req.RequestId
			toPut = append(toPut, b)
			if b.Proto.Output == nil {
				b.Proto.Output = &pb.Build_Output{}
			}
			b.Proto.Output.Status = pb.Status_STARTED
			statusUpdater := buildstatus.Updater{
				Build:        b,
				OutputStatus: &buildstatus.StatusWithDetails{Status: pb.Status_STARTED},
				UpdateTime:   clock.Now(ctx),
				PostProcess:  tasks.SendOnBuildStatusChange,
			}
			var bs *model.BuildStatus
			bs, err = statusUpdater.Do(ctx)
			if err != nil {
				return appstatus.Errorf(codes.Internal, "failed to update status for build %d: %s", b.ID, err)
			}
			if bs != nil {
				buildStatusChanged = true
				toPut = append(toPut, bs)
			}

			if err = datastore.Put(ctx, toPut...); err != nil {
				return appstatus.Errorf(codes.Internal, "failed to start build %d: %s", b.ID, err)
			}
			return nil
		}
		return checkSubsequentRequest(req, b.StartBuildRequestID, infra.Proto.Swarming.TaskId)
	}, nil)
	if txErr != nil {
		return nil, false, txErr
	}
	return b, buildStatusChanged, nil
}

func startBuildOnBackendOnFirstReq(ctx context.Context, req *pb.StartBuildRequest, b *model.Build, infra *model.BuildInfra) (bool, error) {
	taskID := infra.Proto.Backend.Task.GetId()
	if taskID.GetId() != "" && taskID.GetId() != req.TaskId {
		// The build has been associated with another task, possible from a previous
		// RegisterBuildTask call from a different task.
		return false, buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has associated with task %q", req.BuildId, taskID.Id))
	}

	if protoutil.IsEnded(b.Status) {
		// The build has ended.
		// For example the StartBuild request reaches Buildbucket late, when the task
		// has crashed (e.g. BOT_DIED).
		return false, appstatus.Errorf(codes.FailedPrecondition, "cannot start ended build %d", b.ID)
	}

	if b.Status == pb.Status_STARTED {
		// The build has started.
		// Currently for builds on backend this should not happen.
		return false, appstatus.Errorf(codes.FailedPrecondition, "cannot start started build %d", b.ID)
	}

	// Start the build.
	toSave := []any{b}
	b.StartBuildRequestID = req.RequestId
	updateBuildToken, err := buildtoken.GenerateToken(ctx, b.ID, pb.TokenBody_BUILD)
	if err != nil {
		return false, errors.Annotate(err, "failed to generate BUILD token for build %d", b.ID).Err()
	}
	b.UpdateToken = updateBuildToken
	if b.Proto.Output == nil {
		b.Proto.Output = &pb.Build_Output{}
	}
	b.Proto.Output.Status = pb.Status_STARTED
	statusUpdater := buildstatus.Updater{
		Build:        b,
		OutputStatus: &buildstatus.StatusWithDetails{Status: pb.Status_STARTED},
		UpdateTime:   clock.Now(ctx),
		PostProcess:  tasks.SendOnBuildStatusChange,
	}
	var bs *model.BuildStatus
	bs, err = statusUpdater.Do(ctx)
	if err != nil {
		return false, appstatus.Errorf(codes.Internal, "failed to update status for build %d: %s", b.ID, err)
	}
	if bs != nil {
		toSave = append(toSave, bs)
	}

	if taskID.GetId() == "" {
		// First handshake, associate the task with the build.
		taskID.Id = req.TaskId
		toSave = append(toSave, infra)
	}
	err = datastore.Put(ctx, toSave)
	if err != nil {
		return false, errors.Annotate(err, "failed to start build %d: %s", b.ID, err).Err()
	}

	return true, nil
}

func startBuildOnBackend(ctx context.Context, req *pb.StartBuildRequest) (*model.Build, bool, error) {
	var b *model.Build
	buildStatusChanged := false
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		entities, err := common.GetBuildEntities(ctx, req.BuildId, model.BuildKind, model.BuildInfraKind)
		if err != nil {
			return errors.Annotate(err, "failed to get build %d", req.BuildId).Err()
		}
		b = entities[0].(*model.Build)
		infra := entities[1].(*model.BuildInfra)

		if infra.Proto.GetBackend().GetTask() == nil {
			return errors.Reason("the build %d does not run on task backend", req.BuildId).Err()
		}

		if b.StartBuildRequestID == "" {
			// First StartBuild for the build.
			buildStatusChanged, err = startBuildOnBackendOnFirstReq(ctx, req, b, infra)
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

	// a token is required
	rawToken, err, _ := getBuildbucketToken(ctx, false)
	if err != nil {
		return nil, err
	}

	// token can either be BUILD or START_BUILD
	tok, err := buildtoken.ParseToTokenBody(ctx, rawToken, req.BuildId, pb.TokenBody_START_BUILD, pb.TokenBody_BUILD)
	if err != nil {
		return nil, err
	}

	switch tok.Purpose {
	case pb.TokenBody_BUILD:
		b, buildStatusChanged, err = startBuildOnSwarming(ctx, req, rawToken)
	case pb.TokenBody_START_BUILD:
		b, buildStatusChanged, err = startBuildOnBackend(ctx, req)
	default:
		panic(fmt.Sprintf("impossible: invalid token purpose: %s", tok.Purpose))
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
