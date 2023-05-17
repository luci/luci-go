// Copyright 2022 The LUCI Authors.
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
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func validateTaskStatus(taskStatus pb.Status) error {
	switch taskStatus {
	case pb.Status_SCHEDULED,
		pb.Status_ENDED_MASK,
		pb.Status_STATUS_UNSPECIFIED:
		return errors.Reason("task.status: invalid status %s for UpdateBuildTask", taskStatus).Err()
	}

	if _, ok := pb.Status_value[taskStatus.String()]; !ok {
		return errors.Reason("task.status: invalid status %s for UpdateBuildTask", taskStatus).Err()
	}
	return nil
}

func validateUpdateBuildTaskRequest(ctx context.Context, req *pb.UpdateBuildTaskRequest) error {
	if procRes := protowalk.Fields(req, &protowalk.RequiredProcessor{}); procRes != nil {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			return errors.Reason(strings.Join(resStrs, ". ")).Err()
		}
	}
	if req.GetTask().GetId().GetId() == "" {
		return errors.Reason("task.id: required").Err()
	}
	if err := validateTaskStatus(req.Task.Status); err != nil {
		return errors.Annotate(err, "task.Status").Err()
	}
	detailsInKb := float64(len(req.Task.GetDetails().String()) / 1024)
	if detailsInKb > 10 {
		return errors.Reason("task.details is greater than 10 kb").Err()
	}
	return nil
}

// validateBuildTask ensures that the taskID provided in the request matches
// the taskID that is stored in the build model. If there is no task associated
// with the build, the task is associated here.
func validateBuildTask(ctx context.Context, req *pb.UpdateBuildTaskRequest, build *model.Build) (*pb.Build, error) {

	// create a new build mask to read infra
	mask, _ := model.NewBuildMask("", nil, &pb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"id", "infra"},
		},
	})
	bld, err := build.ToProto(ctx, mask, nil)
	if err != nil {
		return nil, errors.Annotate(err, "could not load build details").Err()
	}
	if bld.Infra.GetBackend() == nil {
		bld.Infra.Backend = &pb.BuildInfra_Backend{}
	}
	if bld.Infra.Backend.GetTask() == nil {
		bld.Infra.Backend.Task = &pb.Task{}
	} else if bld.Infra.Backend.Task.Id.GetId() != req.Task.Id.GetId() ||
		bld.Infra.Backend.Task.Id.GetTarget() != req.Task.Id.GetTarget() {
		return nil, errors.Reason("task ID in request does not match task ID associated with build").Err()
	}
	if protoutil.IsEnded(bld.Infra.Backend.Task.Status) {
		return nil, appstatus.Errorf(codes.FailedPrecondition, "cannot update an ended task")
	}
	bld.Infra.Backend.Task = req.Task
	return bld, nil
}

func updateTaskEntity(ctx context.Context, req *pb.UpdateBuildTaskRequest, build *pb.Build) error {
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		b, err := getBuild(ctx, build.Id)
		if err != nil {
			if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
				return err
			}
			return appstatus.Errorf(codes.Internal, "failed to get build %d: %s", build.Id, err)
		}
		bk := datastore.KeyForObj(ctx, b)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: build.Infra,
		}
		toSave := []any{infra}
		return datastore.Put(ctx, toSave)
	}, nil)

	return txErr
	// TODO [randymaldonado@] send pubsub notifications and update metrics.
}

func (*Builds) UpdateBuildTask(ctx context.Context, req *pb.UpdateBuildTaskRequest) (*pb.Task, error) {
	buildID, err := strconv.ParseInt(req.GetBuildId(), 10, 64)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "bad build id").Err())
	}
	_, err = validateToken(ctx, buildID, pb.TokenBody_TASK)
	if err != nil {
		return nil, err
	}

	if err := validateUpdateBuildTaskRequest(ctx, req); err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s", err)
	}
	logging.Infof(ctx, "Received an UpdateBuildTask request for build %q", req.BuildId)

	bld, err := getBuild(ctx, buildID)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "invalid build").Err())
	}

	// Pre-check if the task can be updated before updating it with a transaction.
	// Ensures that the taskID provided in the request matches the taskID that is
	// stored in the build model. If there is no task associated with the build model,
	// the task is associated here in buildInfra.Infra.Backend.Task.
	build, err := validateBuildTask(ctx, req, bld)
	if err != nil {
		return nil, errors.Annotate(err, "invalid task").Err()
	}

	err = updateTaskEntity(ctx, req, build)
	if err != nil {
		if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
			return nil, err
		} else {
			return nil, appstatus.Errorf(codes.Internal, "failed to update the build entity: %s", err)
		}
	}

	return build.Infra.Backend.Task, nil
}
