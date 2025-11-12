// Copyright 2025 The LUCI Authors.
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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"
	executorpb "go.chromium.org/turboci/proto/go/graph/executor/v1"
)

// ValidateStage implements executorgrpcpb.TurboCIStageExecutorServer.
func (*TurboCIStageExecutor) ValidateStage(ctx context.Context, req *executorpb.ValidateStageRequest) (*executorpb.ValidateStageResponse, error) {
	TurboCICall(ctx).LogDetails(ctx)

	if err := validateStage(ctx); err != nil {
		return nil, err
	}
	return &executorpb.ValidateStageResponse{}, nil
}

func validateStage(ctx context.Context) error {
	callInfo := TurboCICall(ctx)
	if callInfo == nil {
		return appstatus.Error(codes.Internal, "missing call info")
	}

	req := callInfo.ScheduleBuild
	if req.GetTemplateBuildId() != 0 {
		return appstatus.BadRequest(fmt.Errorf("Buildbucket stage with template_build_id is not supported"))
	}
	if req.GetParentBuildId() != 0 {
		return appstatus.BadRequest(fmt.Errorf("Buildbucket stage with parent_build_id is not supported"))
	}
	// TODO: b/449231057 - support led usage in the future.
	if req.GetShadowInput() != nil {
		return appstatus.BadRequest(fmt.Errorf("Buildbucket stage with shadow_input is not supported"))
	}

	// TODO: b/449231057 - support parent validation.
	_, _, err := validateScheduleBuild(ctx, nil, req, nil, nil)
	return err
}
