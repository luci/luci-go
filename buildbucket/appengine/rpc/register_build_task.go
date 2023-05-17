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

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

type RegisterBuildTaskChecker struct{}

var _ protowalk.FieldProcessor = (*RegisterBuildTaskChecker)(nil)

func (*RegisterBuildTaskChecker) Process(field protoreflect.FieldDescriptor, _ protoreflect.Message) (data protowalk.ResultData, applied bool) {
	cbfb := proto.GetExtension(field.Options().(*descriptorpb.FieldOptions), pb.E_RegisterBuildTaskFieldOption).(*pb.RegisterBuildTaskFieldOption)
	switch cbfb.FieldBehavior {
	case annotations.FieldBehavior_REQUIRED:
		return protowalk.ResultData{Message: "required", IsErr: true}, true
	default:
		panic("unsupported field behavior")
	}
}

func init() {
	protowalk.RegisterFieldProcessor(&RegisterBuildTaskChecker{}, func(field protoreflect.FieldDescriptor) protowalk.ProcessAttr {
		if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
			if cbfb := proto.GetExtension(fo, pb.E_RegisterBuildTaskFieldOption).(*pb.RegisterBuildTaskFieldOption); cbfb != nil {
				switch cbfb.FieldBehavior {
				case annotations.FieldBehavior_REQUIRED:
					return protowalk.ProcessIfUnset
				default:
					panic("unsupported field behavior")
				}
			}
		}
		return protowalk.ProcessNever
	})
}

func validateRegisterBuildTaskRequest(ctx context.Context, req *pb.RegisterBuildTaskRequest) error {
	if procRes := protowalk.Fields(req, &protowalk.RequiredProcessor{}, &RegisterBuildTaskChecker{}); procRes != nil {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			logging.Infof(ctx, strings.Join(resStrs, ". "))
		}
		return procRes.Err()
	}
	return nil
}

func registerBuildTask(ctx context.Context, req *pb.RegisterBuildTaskRequest, b *model.Build, infra *model.BuildInfra) (string, error) {
	taskID := infra.Proto.Backend.Task.GetId()
	if taskID.GetTarget() != req.Task.Id.Target {
		return "", appstatus.BadRequest(errors.Reason("build %d requires task target %q, got %q", b.ID, taskID.GetTarget(), req.Task.Id.Target).Err())
	}

	if taskID.GetId() != "" && taskID.GetId() != req.Task.Id.Id {
		// The build has been associated with another task, possible from a previous
		// StartBuild call of the bbagent from a different task.
		return "", buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has associated with task %q", req.BuildId, taskID.Id))
	}

	// Either the build has not associated with any task yet, or it has associated
	// with the same task as this request, possibly from a previous StartBuild
	// call of the bbagent from the same task.
	// In either case,
	// * set the build's backend task,
	// * store the request_id,
	// * generate a new TASK token, store it and return it to caller.
	infra.Proto.Backend.Task = req.Task
	b.RegisterTaskRequestID = req.RequestId
	updateTaskToken, err := buildtoken.GenerateToken(ctx, b.ID, pb.TokenBody_TASK)
	if err != nil {
		return "", appstatus.Errorf(codes.Internal, "failed to generate TASK token for build %d: %s", b.ID, err)
	}
	b.BackendTaskToken = updateTaskToken
	err = datastore.Put(ctx, []any{b, infra})
	if err != nil {
		return "", appstatus.Errorf(codes.Internal, "failed to register backend task to build %d: %s", b.ID, err)
	}
	return updateTaskToken, nil
}

// RegisterBuildTask handles a request to register a TaskBackend task with the build it runs. Implements pb.BuildsServer.
func (*Builds) RegisterBuildTask(ctx context.Context, req *pb.RegisterBuildTaskRequest) (*pb.RegisterBuildTaskResponse, error) {
	if err := validateRegisterBuildTaskRequest(ctx, req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	_, err := validateToken(ctx, req.BuildId, pb.TokenBody_REGISTER_TASK)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "invalid register build task token").Err())
	}

	var updateTaskToken string
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		b, infra, err := getBuildAndInfra(ctx, req.BuildId)
		if err != nil {
			if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
				return err
			}
			return appstatus.Errorf(codes.Internal, "failed to get build %d: %s", req.BuildId, err)
		}

		if infra.Proto.GetBackend().GetTask() == nil {
			return appstatus.Errorf(codes.Internal, "the build %d does not run on task backend", req.BuildId)
		}

		if b.RegisterTaskRequestID == "" {
			// First RegisterBuildTask for the build.
			// Update the build to register the task.
			updateTaskToken, err = registerBuildTask(ctx, req, b, infra)
			return err
		}

		if b.RegisterTaskRequestID != req.RequestId {
			// Different request id, deduplicate.
			return buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has recorded another RegisterBuildTask with request id %q", req.BuildId, b.RegisterTaskRequestID))
		}

		if infra.Proto.Backend.Task.GetId().GetTarget() == req.Task.Id.Target && infra.Proto.Backend.Task.GetId().GetId() == req.Task.Id.Id {
			// Idempotent.
			updateTaskToken = b.BackendTaskToken
			return nil
		}

		// Same request id, different task id.
		return buildbucket.TaskWithCollidedRequestID.Apply(appstatus.Errorf(codes.Internal, "build %d has associated with task %q with RegisterBuildTask request id %q", req.BuildId, infra.Proto.Backend.Task.Id, b.RegisterTaskRequestID))
	}, nil)
	if txErr != nil {
		return nil, txErr
	}
	return &pb.RegisterBuildTaskResponse{UpdateBuildTaskToken: updateTaskToken}, nil
}
