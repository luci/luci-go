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

	"go.chromium.org/luci/grpc/appstatus"
	executorpb "go.chromium.org/turboci/proto/go/graph/executor/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// ValidateStage implements executorgrpcpb.TurboCIStageExecutorServer.
func (*TurboCIStageExecutor) ValidateStage(ctx context.Context, req *executorpb.ValidateStageRequest) (*executorpb.ValidateStageResponse, error) {
	TurboCICall(ctx).LogDetails(ctx)

	updatedPolicy, err := validateStage(ctx, req.GetStage())
	if err != nil {
		return nil, err
	}

	return executorpb.ValidateStageResponse_builder{
		StageExecutionPolicy: updatedPolicy,
	}.Build(), nil
}

func validateStage(ctx context.Context, stage *orchestratorpb.Stage) (*orchestratorpb.StageExecutionPolicy, error) {
	req := TurboCICall(ctx).ScheduleBuild
	if req.GetTemplateBuildId() != 0 {
		return nil, appstatus.BadRequest(fmt.Errorf("Buildbucket stage with template_build_id is not supported"))
	}
	if req.GetParentBuildId() != 0 {
		return nil, appstatus.BadRequest(fmt.Errorf("Buildbucket stage with parent_build_id is not supported"))
	}

	// Stage execution policy
	// The timeout fields in req must be unset.
	if req.GetExecutionTimeout() != nil {
		return nil, appstatus.BadRequest(fmt.Errorf("Buildbucket stage args must unset execution_timeout"))
	}
	if req.GetSchedulingTimeout() != nil {
		return nil, appstatus.BadRequest(fmt.Errorf("Buildbucket stage args must unset scheduling_timeout"))
	}
	if req.GetGracePeriod() != nil {
		return nil, appstatus.BadRequest(fmt.Errorf("Buildbucket stage args must unset grace_period"))
	}

	// Fill the timeout fields with requested stage execution policy, so it
	// could be combined with configuration in schedule_build.setTimeouts.
	reqPolicy := stage.GetExecutionPolicy().GetRequested()
	fillScheduleBuildRequestWithPolicy(req, reqPolicy.GetAttemptExecutionPolicyTemplate().GetTimeout())

	// Set DryRun to true so the request is only being validated.
	req.DryRun = true

	pBld, err := getParentViaStage(ctx, stage)
	if err != nil {
		return nil, err
	}

	blds, merr := scheduleBuilds(
		ctx,
		[]*pb.ScheduleBuildRequest{req},
		&scheduleBuildsParams{
			OverrideParent: pBld,
		})
	if merr[0] != nil {
		return nil, merr[0]
	}

	updatedPolicy := buildToStageExecutionPolicy(blds[0], reqPolicy)
	return updatedPolicy, nil
}
