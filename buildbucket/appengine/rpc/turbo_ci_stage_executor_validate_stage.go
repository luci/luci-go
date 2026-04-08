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
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	executorpb "go.chromium.org/turboci/proto/go/graph/executor/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// ValidateStage implements executorgrpcpb.TurboCIStageExecutorServer.
func (*TurboCIStageExecutor) ValidateStage(ctx context.Context, req *executorpb.ValidateStageRequest) (*executorpb.ValidateStageResponse, error) {
	call := TurboCICall(ctx)
	call.LogDetails(ctx)

	updatedPolicy, taskAccount, err := validateStage(ctx, req.GetStage(), call.ScheduleBuild)
	if err != nil {
		return nil, err
	}

	projectAccount, err := lookupProjectAccount(ctx, call.ScheduleBuild.Builder.Project)
	if err != nil {
		return nil, err
	}

	return executorpb.ValidateStageResponse_builder{
		StageExecutionPolicy: updatedPolicy,
		StageServiceAccounts: []string{
			// To allow Buildbucket server to cleanup after e.g. crashed build.
			projectAccount,
			// For Turbo CI calls from the build itself.
			taskAccount,
		},
	}.Build(), nil
}

func validateStage(ctx context.Context, stage *orchestratorpb.Stage, req *pb.ScheduleBuildRequest) (policy *orchestratorpb.StageExecutionPolicy, taskAccount string, err error) {
	if req.GetTemplateBuildId() != 0 {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage with template_build_id is not supported"))
	}
	if req.GetParentBuildId() != 0 {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage with parent_build_id is not supported"))
	}

	// Stage execution policy
	// The timeout fields in req must be unset.
	if req.GetExecutionTimeout() != nil {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage args must unset execution_timeout"))
	}
	if req.GetSchedulingTimeout() != nil {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage args must unset scheduling_timeout"))
	}
	if req.GetGracePeriod() != nil {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage args must unset grace_period"))
	}

	if req.GetDryRun() {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage with dry_run is not supported"))
	}

	// Fill the timeout fields with requested stage execution policy, so it
	// could be combined with configuration in schedule_build.setTimeouts.
	reqPolicy := stage.GetExecutionPolicy().GetRequested()
	fillScheduleBuildRequestWithPolicy(req, reqPolicy.GetAttemptExecutionPolicyTemplate().GetTimeout())

	// Set DryRun to true so the request is only being validated.
	req.DryRun = true

	pBld, err := getParentViaStage(ctx, stage)
	if err != nil {
		return nil, "", err
	}

	blds, merr := scheduleBuilds(
		ctx,
		[]*pb.ScheduleBuildRequest{req},
		&scheduleBuildsParams{
			OverrideParent: pBld,
		})
	if merr[0] != nil {
		return nil, "", merr[0]
	}

	updatedPolicy := buildToStageExecutionPolicy(blds[0], reqPolicy)
	taskAccount = taskServiceAccount(blds[0].Infra)
	if taskAccount == "" {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage builder doesn't have a service account configured: it is requires to call Turbo CI APIs"))
	}

	return updatedPolicy, taskAccount, nil
}

func lookupProjectAccount(ctx context.Context, project string) (string, error) {
	data, err := auth.GetRealmData(ctx, realms.Join(project, realms.RootRealm))
	if err != nil {
		return "", appstatus.Errorf(codes.Internal, "error looking up realm data")
	}
	account := data.GetProjectScopedAccount()
	if account == "" {
		return "", appstatus.BadRequest(fmt.Errorf("unrecognized or misconfigured LUCI project %q, it doesn't have a project-scoped account", project))
	}
	return account, nil
}
