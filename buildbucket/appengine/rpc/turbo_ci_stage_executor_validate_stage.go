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
	"google.golang.org/protobuf/proto"

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

	policy, taskAccount, err := validateStage(ctx, req.GetStage(), call.ScheduleBuild)
	if err != nil {
		return nil, err
	}

	projectAccount, err := lookupProjectAccount(ctx, call.ScheduleBuild.Builder.Project)
	if err != nil {
		return nil, err
	}

	return executorpb.ValidateStageResponse_builder{
		StageExecutionPolicy: policy,
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
	if req.GetMask() != nil || req.GetFields() != nil {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage with field masks is not supported"))
	}
	if req.GetDryRun() {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage with dry_run is not supported"))
	}

	// The timeout fields in req must be unset, they should be passed via Turbo CI
	// execution policy instead.
	if req.GetExecutionTimeout() != nil {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage args must unset execution_timeout"))
	}
	if req.GetSchedulingTimeout() != nil {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage args must unset scheduling_timeout"))
	}
	if req.GetGracePeriod() != nil {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage args must unset grace_period"))
	}

	// Buildbucket executor doesn't support multiple attempts currently. A stage
	// is bolted to a single concrete build ID representing one single attempt.
	policy = stage.GetExecutionPolicy().GetRequested()
	if policy.GetRetry() != nil {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket doesn't support retrying stage attempts"))
	}

	// Put requested timeouts (if any) back into ScheduleBuildRequest to pass them
	// to the Buildbucket guts to be joined with the builder config. This is a
	// reverse of scheduleBuildRequestToStage(...).
	scheduleBuildRequestFromPolicyTimeout(req, policy.GetAttemptExecutionPolicyTemplate().GetTimeout())

	// Set DryRun to true so the request is only being validated.
	req.DryRun = true

	pBld, err := getParentViaStage(ctx, stage)
	if err != nil {
		return nil, "", err
	}

	// Validate the request (including ACLs) and fully expand it into a Build
	// based on the builder config.
	blds, merr := scheduleBuilds(
		ctx,
		[]*pb.ScheduleBuildRequest{req},
		&scheduleBuildsParams{
			OverrideParent: pBld,
			LaunchAsNative: true,
		})
	if merr[0] != nil {
		return nil, "", merr[0]
	}
	build := blds[0]

	// Get the account the build runs as to bound a stage attempt token to it.
	taskAccount = taskServiceAccount(build.Infra)
	if taskAccount == "" {
		return nil, "", appstatus.BadRequest(fmt.Errorf("Buildbucket stage builder doesn't have a service account configured: it is requires to call Turbo CI APIs"))
	}

	// Prepare the first draft of the policy that will be enforced by the
	// Turbo CI, to show it in the API responses while the stage is PLANNED. We'll
	// potentially adjust it when moving the stage to SCHEDULED/RUNNING state
	// (since the builder config may change by that time).
	policy = proto.CloneOf(policy)
	if policy == nil {
		policy = &orchestratorpb.StageExecutionPolicy{}
	}
	policy.SetAttemptExecutionPolicyTemplate(buildToAttemptExecutionPolicy(build))

	return policy, taskAccount, nil
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
