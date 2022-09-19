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
	"strings"

	"google.golang.org/grpc/codes"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/protowalk"
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

// ensures that the taskID provided in the request matches the taskID that is
// stored in the build model. If there is no task associated with the build,
// the task is associated here.
func validateBuildTask(ctx context.Context, req *pb.UpdateBuildTaskRequest, build *model.Build) (*pb.Build, error) {

	// create a new build mask to read infra
	mask, _ := model.NewBuildMask("", nil, &pb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"infra"},
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
		return nil, errors.Reason("task id in reuqest does not match task id associated with build").Err()
	}
	if protoutil.IsEnded(bld.Infra.Backend.Task.Status) {
		return nil, appstatus.Errorf(codes.FailedPrecondition, "cannot update an ended task")
	}
	bld.Infra.Backend.Task = req.Task
	return bld, nil
}

func (*Builds) UpdateBuildTask(ctx context.Context, req *pb.UpdateBuildTaskRequest) (*pb.Task, error) {
	if err := validateUpdateBuildTaskRequest(ctx, req); err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s", err)
	}

	_, bld, err := validateBuildTaskToken(ctx, req.GetBuildId())
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "invalid build").Err())
	}

	// pre-check if the task can be updated before updating it with a transaction.
	// ensures that the taskID provided in the request matches the taskID that is
	// stored in the build model. If there is no task associated with the build model,
	// the task is associated here in buildInfra.Infra.Backend.Task
	build, err := validateBuildTask(ctx, req, bld)
	if err != nil {
		return nil, errors.Annotate(err, "invalid task").Err()
	}

	return build.Infra.Backend.Task, nil
}
