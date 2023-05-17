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
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
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

// getBuildInfraStatus returns the build, its infra and status entity with the
// given ID or NotFound appstatus if any entity is not found.
func getBuildInfraStatus(ctx context.Context, id int64) (*model.Build, *model.BuildInfra, *model.BuildStatus, error) {
	bld := &model.Build{ID: id}
	infra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, &model.Build{ID: id})}
	bs := &model.BuildStatus{Build: datastore.KeyForObj(ctx, &model.Build{ID: id})}
	switch err := datastore.Get(ctx, bld, infra, bs); {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		merr, ok := err.(errors.MultiError)
		if ok && merr[0] == nil && merr[1] == nil && merr[2] == datastore.ErrNoSuchEntity {
			// This is allowed during BuildStatus rollout.
			// TODO(crbug.com/1430324): also disallow ErrNoSuchEntity for bs.
			return bld, infra, nil, nil
		}
		return nil, nil, nil, perm.NotFoundErr(ctx)
	case err != nil:
		return nil, nil, nil, errors.Annotate(err, "error fetching build or infra with ID %d", id).Err()
	default:
		return bld, infra, bs, nil
	}
}

// startBuildOnSwarming starts a build if it runs on swarming.
//
// For builds on Swarming, the swarming task id and update_token have been
// saved in datastore during task creation.
func startBuildOnSwarming(ctx context.Context, req *pb.StartBuildRequest) (*model.Build, bool, error) {
	var b *model.Build
	buildStatusChanged := false
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var infra *model.BuildInfra
		var bs *model.BuildStatus
		var err error
		b, infra, bs, err = getBuildInfraStatus(ctx, req.BuildId)
		if err != nil {
			return errors.Annotate(err, "failed to get build %d", req.BuildId).Err()
		}

		if infra.Proto.GetSwarming() == nil {
			return appstatus.Errorf(codes.Internal, "the build %d does not run on swarming", req.BuildId)
		}

		// First StartBuild request.
		if b.StartBuildRequestID == "" {
			if infra.Proto.Swarming.TaskId == req.TaskId {
				if protoutil.IsEnded(b.Status) {
					// The build has ended.
					// For example the StartBuild request reaches Buildbucket late, when the task
					// has crashed (e.g. BOT_DIED).
					return appstatus.Errorf(codes.FailedPrecondition, "cannot start ended build %d", b.ID)
				}

				// Start the build.
				toPut := []any{b}
				b.StartBuildRequestID = req.RequestId
				if b.Status == pb.Status_SCHEDULED {
					// Should only set the build to STARTED if it's still pending.
					// Because of the swarming sync flow, it's possible that build is
					// already in STARTED status.
					protoutil.SetStatus(clock.Now(ctx), b.Proto, pb.Status_STARTED)
					buildStatusChanged = true
					if bs != nil {
						bs.Status = pb.Status_STARTED
						toPut = append(toPut, bs)
					}
				}
				if err = datastore.Put(ctx, toPut...); err != nil {
					return appstatus.Errorf(codes.Internal, "failed to start build %d: %s", b.ID, err)
				}

				if buildStatusChanged {
					// Notify Pubsub.
					// NotifyPubSub is a Transactional task, so do it inside the transaction.
					_ = tasks.NotifyPubSub(ctx, b)
				}
				return nil
			}

			// Duplicated task.
			return buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has associated with task %q", req.BuildId, infra.Proto.Swarming.TaskId))
		}
		return checkSubsequentRequest(req, b.StartBuildRequestID, infra.Proto.Swarming.TaskId)
	}, nil)
	if txErr != nil {
		return nil, false, txErr
	}
	return b, buildStatusChanged, nil
}

func startBuildOnBackendOnFirstReq(ctx context.Context, req *pb.StartBuildRequest, b *model.Build, infra *model.BuildInfra, bs *model.BuildStatus) (bool, error) {
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
	b.StartBuildRequestID = req.RequestId
	updateBuildToken, err := buildtoken.GenerateToken(ctx, b.ID, pb.TokenBody_BUILD)
	if err != nil {
		return false, errors.Annotate(err, "failed to generate BUILD token for build %d", b.ID).Err()
	}
	b.UpdateToken = updateBuildToken
	protoutil.SetStatus(clock.Now(ctx), b.Proto, pb.Status_STARTED)
	toSave := []any{b}
	if bs != nil {
		bs.Status = pb.Status_STARTED
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

	// Notify Pubsub.
	// NotifyPubSub is a Transactional task, so do it inside the transaction.
	_ = tasks.NotifyPubSub(ctx, b)

	return true, nil
}

func startBuildOnBackend(ctx context.Context, req *pb.StartBuildRequest) (*model.Build, bool, error) {
	var b *model.Build
	buildStatusChanged := false
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var infra *model.BuildInfra
		var bs *model.BuildStatus
		var err error
		b, infra, bs, err = getBuildInfraStatus(ctx, req.BuildId)
		if err != nil {
			return errors.Annotate(err, "failed to get build %d", req.BuildId).Err()
		}

		if infra.Proto.GetBackend().GetTask() == nil {
			return errors.Reason("the build %d does not run on task backend", req.BuildId).Err()
		}

		if b.StartBuildRequestID == "" {
			// First StartBuild for the build.
			buildStatusChanged, err = startBuildOnBackendOnFirstReq(ctx, req, b, infra, bs)
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
	rawToken, err := getBuildbucketToken(ctx, false)
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
		b, buildStatusChanged, err = startBuildOnSwarming(ctx, req)
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
