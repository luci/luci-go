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

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

type StartBuildTaskChecker struct{}

var _ protowalk.FieldProcessor = (*StartBuildTaskChecker)(nil)

func (*StartBuildTaskChecker) Process(field protoreflect.FieldDescriptor, _ protoreflect.Message) (data protowalk.ResultData, applied bool) {
	cbfb := proto.GetExtension(field.Options().(*descriptorpb.FieldOptions), pb.E_StartBuildTaskFieldOption).(*pb.StartBuildTaskFieldOption)
	switch cbfb.FieldBehavior {
	case annotations.FieldBehavior_REQUIRED:
		return protowalk.ResultData{Message: "required", IsErr: true}, true
	default:
		panic("unsupported field behavior")
	}
}

func init() {
	protowalk.RegisterFieldProcessor(&StartBuildTaskChecker{}, func(field protoreflect.FieldDescriptor) protowalk.ProcessAttr {
		if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
			if cbfb := proto.GetExtension(fo, pb.E_StartBuildTaskFieldOption).(*pb.StartBuildTaskFieldOption); cbfb != nil {
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

func validateStartBuildTaskRequest(ctx context.Context, req *pb.StartBuildTaskRequest) error {
	if procRes := protowalk.Fields(req, &protowalk.RequiredProcessor{}, &StartBuildTaskChecker{}); procRes != nil {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			logging.Infof(ctx, strings.Join(resStrs, ". "))
		}
		return procRes.Err()
	}
	return nil
}

func computeBackendPubsubTopic(ctx context.Context, target string) (string, error) {
	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return "", errors.Annotate(err, "could not get global settings config").Err()
	}
	if globalCfg == nil {
		return "", errors.Reason("error fetching service config").Err()
	}
	for _, backend := range globalCfg.Backends {
		if backend.Target == target {
			return fmt.Sprintf("projects/%s/topics/%s", info.AppID(ctx), backend.PubsubId), nil
		}
	}
	return "", errors.Reason("pubsub id not found").Err()
}

// startBuildTask implements the core logic of registering and/or starting a build task.
// The startBuildToken is generated for the task here.
func startBuildTask(ctx context.Context, req *pb.StartBuildTaskRequest, b *model.Build, infra *model.BuildInfra) (string, error) {
	taskID := infra.Proto.Backend.Task.GetId()
	if taskID.GetTarget() != req.Task.Id.Target {
		return "", appstatus.BadRequest(errors.Reason("build %d requires task target %q, got %q", b.ID, taskID.GetTarget(), req.Task.Id.Target).Err())
	}

	if taskID.GetId() != "" && taskID.GetId() != req.Task.Id.Id {
		// The build has been associated with another task from the RunTaskResponse
		// or another StartBuildTask call.
		return "", buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has associated with task %q", req.BuildId, taskID.Id))
	}

	// Either the build has not associated with any task yet, or it has associated
	// with the same task as this request, possibly from a RunTaskResponse
	// In either case,
	// * set the build's backend task,
	// * store the request_id,
	// * generate a new START_BUILD token, store it and return it to caller.
	infra.Proto.Backend.Task = req.Task
	b.StartBuildTaskRequestID = req.RequestId
	startBuildToken, err := buildtoken.GenerateToken(ctx, b.ID, pb.TokenBody_START_BUILD)
	if err != nil {
		return "", appstatus.Errorf(codes.Internal, "failed to generate START_BUILD token for build %d: %s", b.ID, err)
	}
	b.StartBuildToken = startBuildToken
	err = datastore.Put(ctx, []any{b, infra})
	if err != nil {
		return "", appstatus.Errorf(codes.Internal, "failed to start backend task to build %d: %s", b.ID, err)
	}
	return startBuildToken, nil
}

// StartBuildTask handles a request to register a TaskBackend task with the build it runs. Implements pb.BuildsServer.
func (*Builds) StartBuildTask(ctx context.Context, req *pb.StartBuildTaskRequest) (*pb.StartBuildTaskResponse, error) {
	if err := validateStartBuildTaskRequest(ctx, req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	_, err := validateToken(ctx, req.BuildId, pb.TokenBody_START_BUILD_TASK)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "invalid register build task token").Err())
	}

	var startBuildToken string
	var resultdbInvocationUpdateToken string
	var pubsubTopic string
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		entities, err := common.GetBuildEntities(ctx, req.BuildId, model.BuildKind, model.BuildInfraKind)
		if err != nil {
			if _, isAppStatusErr := appstatus.Get(err); isAppStatusErr {
				return err
			}
			return appstatus.Errorf(codes.Internal, "failed to get build %d: %s", req.BuildId, err)
		}
		b := entities[0].(*model.Build)
		infra := entities[1].(*model.BuildInfra)

		resultdbInvocationUpdateToken = b.ResultDBUpdateToken

		if infra.Proto.GetBackend().GetTask() == nil {
			return appstatus.Errorf(codes.Internal, "the build %d does not run on task backend", req.BuildId)
		}

		if b.StartBuildTaskRequestID == "" {
			// First StartBuildTask for the build.
			// Update the build to register the task.
			startBuildToken, err = startBuildTask(ctx, req, b, infra)
			if err != nil {
				return err
			}
			pubsubTopic, err = computeBackendPubsubTopic(ctx, infra.Proto.Backend.Task.Id.Target)
			return err
		}

		if b.StartBuildTaskRequestID != req.RequestId {
			// Different request id, deduplicate.
			return buildbucket.DuplicateTask.Apply(appstatus.Errorf(codes.AlreadyExists, "build %d has recorded another StartBuildTask with request id %q", req.BuildId, b.StartBuildTaskRequestID))
		}

		if infra.Proto.Backend.Task.GetId().GetTarget() == req.Task.Id.Target && infra.Proto.Backend.Task.GetId().GetId() == req.Task.Id.Id {
			startBuildToken = b.StartBuildToken
			pubsubTopic, err = computeBackendPubsubTopic(ctx, infra.Proto.Backend.Task.Id.Target)
			return err
		}

		// Same request id, different task id.
		return buildbucket.TaskWithCollidedRequestID.Apply(appstatus.Errorf(codes.Internal, "build %d has associated with task %q with StartBuildTask request id %q", req.BuildId, infra.Proto.Backend.Task.Id, b.StartBuildTaskRequestID))
	}, nil)
	if txErr != nil {
		return nil, txErr
	}

	return &pb.StartBuildTaskResponse{
		Secrets: &pb.BuildSecrets{
			StartBuildToken:               startBuildToken,
			ResultdbInvocationUpdateToken: resultdbInvocationUpdateToken,
		},
		PubsubTopic: pubsubTopic,
	}, nil
}
