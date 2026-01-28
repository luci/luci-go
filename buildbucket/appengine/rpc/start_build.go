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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/turboci/id"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildstatus"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
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

func startBuildOnBackendOnFirstReq(ctx context.Context, req *pb.StartBuildRequest, b *model.Build, infra *model.BuildInfra) (bool, error) {
	taskID := infra.Proto.Backend.Task.GetId()
	if taskID.GetId() != "" && taskID.GetId() != req.TaskId {
		// The build has been associated with another task, possible from a previous
		// StartBuild call from a different task.
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
		return false, errors.Fmt("failed to generate BUILD token for build %d: %w", b.ID, err)
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
		return false, errors.Fmt("failed to start build %d: %s: %w", b.ID, err, err)
	}

	return true, nil
}

type startBuildResult struct {
	bld                *model.Build
	infra              *model.BuildInfra
	buildStatusChanged bool
}

func startBuildOnBackend(ctx context.Context, req *pb.StartBuildRequest) (startBuildResult, error) {
	var b *model.Build
	var infra *model.BuildInfra
	buildStatusChanged := false
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Reset buildStatusChanged in case the transaction callback is retried.
		buildStatusChanged = false
		entities, err := common.GetBuildEntities(ctx, req.BuildId, model.BuildKind, model.BuildInfraKind)
		if err != nil {
			return errors.Fmt("failed to get build %d: %w", req.BuildId, err)
		}
		b = entities[0].(*model.Build)
		infra = entities[1].(*model.BuildInfra)

		if infra.Proto.GetBackend().GetTask() == nil {
			return appstatus.Errorf(codes.FailedPrecondition, "the build %d does not run on task backend", req.BuildId)
		}

		if b.StartBuildRequestID == "" {
			// First StartBuild for the build.
			buildStatusChanged, err = startBuildOnBackendOnFirstReq(ctx, req, b, infra)
			return err
		}

		return checkSubsequentRequest(req, b.StartBuildRequestID, infra.Proto.Backend.Task.GetId().GetId())
	}, nil)
	if txErr != nil {
		// On transient errors, we'll ask bbagent to retry without communicating
		// with TurboCI, so no need to return b.
		if errIsTransient(txErr) {
			return startBuildResult{}, txErr
		}
		// On fatal error we need to set the build stage attempt to INCOMPLETE
		// in TurboCI, return the entities to do that.
		return startBuildResult{bld: b, infra: infra, buildStatusChanged: false}, txErr
	}
	return startBuildResult{bld: b, infra: infra, buildStatusChanged: buildStatusChanged}, nil
}

func checkSubsequentRequest(req *pb.StartBuildRequest, savedReqID, savedTaskID string) error {
	// Subsequent StartBuild request.
	if savedReqID != req.RequestId {
		// Different request id, deduplicate.
		return buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has recorded another StartBuild with request id %q", req.BuildId, savedReqID))
	}

	if savedTaskID != req.TaskId {
		// Same request id, different task id.
		return buildbucket.TaskWithCollidedRequestID.Apply(appstatus.Errorf(codes.InvalidArgument,
			"build %d has associated with task id %q with StartBuild request id %q", req.BuildId, savedTaskID, savedReqID))
	}

	// Idempotent
	return nil
}

// StartBuild handles a request to start a build. Implements pb.BuildsServer.
func (b *Builds) StartBuild(ctx context.Context, req *pb.StartBuildRequest) (*pb.StartBuildResponse, error) {
	if err := validateStartBuildRequest(ctx, req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// a token is required
	rawToken, err, _ := getBuildbucketToken(ctx, false)
	if err != nil {
		return nil, err
	}

	tok, err := buildtoken.ParseToTokenBody(ctx, rawToken, req.BuildId, pb.TokenBody_START_BUILD)
	if err != nil {
		return nil, err
	}
	if tok.Purpose != pb.TokenBody_START_BUILD {
		return nil, appstatus.Errorf(codes.Unauthenticated, "impossible: invalid token purpose: %s", tok.Purpose)
	}

	res, err := startBuildOnBackend(ctx, req)
	// Let bbagent retry if err is transient.
	if errIsTransient(err) {
		return nil, err
	}
	bld := res.bld

	// Report the state change metric before touching Turbo CI
	// to make sure we record the metric correctly even if
	// the Turbo CI call below fails.
	if err == nil && res.buildStatusChanged {
		logging.Infof(ctx, "Build %d: started", bld.ID)
		metrics.BuildStarted(ctx, bld)
	}

	// Here 'startBuildOnBackend' either succeeded or failed with
	// a fatal error. Notify Turbo CI.
	if bld != nil && bld.StageAttemptID != "" {
		if notifyErr := notifyTurboCI(ctx, res, err); notifyErr != nil {
			return nil, notifyErr
		}
	}

	// Done notifying TurboCI, return the original error.
	if err != nil {
		return nil, err
	}

	mask, err := model.NewBuildMask("", nil, &pb.BuildMask{AllFields: true})
	if err != nil {
		return nil, errors.Fmt("failed to construct build mask: %w", err)
	}
	bp, err := bld.ToProto(ctx, mask, nil)
	if err != nil {
		return nil, errors.Fmt("failed to generate build proto from model: %w", err)
	}

	return &pb.StartBuildResponse{Build: bp, UpdateBuildToken: bld.UpdateToken}, nil
}

// notifyTurboCI marks the attempt as either RUNNING on INCOMPLETE depending
// on `startErr`.
//
// If to RUNNING but failed, use grpcutil.WrapIfTransient to wrap the error
// and return it so that bbagent could retry or stop the build accordingly.
//
// If to INCOMPLETE but failed,
//   - On transient error return it to trigger bbagent retry.
//   - On fatal error log it but don't return it. So the original startErr can
//     be returned to bbagent.
func notifyTurboCI(ctx context.Context, res startBuildResult, startErr error) error {
	var cl *turboci.Client
	bld := res.bld
	creds, err := turboci.ProjectRPCCredentials(ctx, bld.Project)
	if err != nil {
		return err
	}
	cl = &turboci.Client{
		Creds: creds,
		Token: bld.StageAttemptToken,
	}

	bp := proto.Clone(bld.Proto).(*pb.Build)
	if res.infra != nil {
		bp.Infra = res.infra.Proto
	}

	if startErr != nil {
		aID, aErr := id.FromString(bld.StageAttemptID)
		if aErr != nil {
			logging.Errorf(ctx, "Build %d has a malformed StageAttemptID: %s", bld.ID, aErr)
			return nil
		}
		err = cl.FailCurrentAttempt(ctx, aID.GetStageAttempt(), &turboci.AttemptFailure{Err: startErr}, tasks.PopulateBuildDetails(bp)...)
		if err != nil && !errIsTransient(err) {
			logging.Errorf(ctx, "Failed to update TurboCI on failure to start build %d (attempt %s): %s", bld.ID, bld.StageAttemptID, err)
			return nil
		}
		return err
	}

	return grpcutil.WrapIfTransient(updateStageAttemptToRunning(ctx, cl, bp, bld.StartBuildRequestID))
}

func errIsTransient(err error) bool {
	return grpcutil.IsTransientCode(grpcutil.Code(err))
}
