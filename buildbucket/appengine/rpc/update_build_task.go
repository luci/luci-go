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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/buildbucket/proto"
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

func (*Builds) UpdateBuildTask(ctx context.Context, req *pb.UpdateBuildTaskRequest) (*pb.Task, error) {
	if err := validateUpdateBuildTaskRequest(ctx, req); err != nil {
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s", err)
	}

	_, _, err := validateBuildTaskToken(ctx, req.GetBuildId())
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "invalid build").Err())
	}

	return req.GetTask(), nil
}
