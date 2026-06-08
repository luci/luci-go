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
	"crypto/sha256"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/turboci/id"
	executorpb "go.chromium.org/turboci/proto/go/graph/executor/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// RunStage implements executorgrpcpb.TurboCIStageExecutorServer.
func (se *TurboCIStageExecutor) RunStage(ctx context.Context, req *executorpb.RunStageRequest) (*executorpb.RunStageResponse, error) {
	TurboCICall(ctx).LogDetails(ctx)

	// Validate the Turbo CI sent a valid request per its API contract. This
	// should never really fail.
	stage := req.GetStage()
	attemptID := req.GetAttempt()
	attemptIDStr := id.ToString(attemptID)
	var attempt *orchestratorpb.Stage_Attempt
	for _, a := range stage.GetAttempts() {
		if proto.Equal(a.GetIdentifier(), attemptID) {
			attempt = a
			break
		}
	}
	if attempt == nil {
		return nil, appstatus.BadRequest(fmt.Errorf("attempt %s not found", attemptIDStr))
	}
	if attempt.GetState() != orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_PENDING {
		return nil, appstatus.BadRequest(fmt.Errorf("attempt %s is in state %s, expecting PENDING", attemptIDStr, attempt.GetState()))
	}
	if req.GetStageAttemptToken() == "" {
		return nil, appstatus.BadRequest(fmt.Errorf("missing stage_attempt_token"))
	}

	// Beyond this point, Buildbucket needs to call WriteNodes to update the
	// stage attempt to TurboCI.

	schReq := TurboCICall(ctx).ScheduleBuild
	creds, err := turboci.ProjectRPCCredentials(ctx, schReq.GetBuilder().GetProject())
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "project credentials: %s", err)
	}
	cl := &turboci.Client{
		Creds: creds,
		Plan:  attempt.GetIdentifier().GetStage().GetWorkPlan(),
		Token: req.GetStageAttemptToken(),
	}

	pBld, err := getParentViaStage(ctx, stage)
	if err != nil {
		if grpcutil.IsTransientCode(appstatus.Code(err)) {
			// Let TurboCI retry on transient errors.
			return nil, err
		}
		return &executorpb.RunStageResponse{}, cl.FailAttempt(ctx, &turboci.AttemptFailure{Err: err})
	}

	// Put requested timeouts (if any) back into ScheduleBuildRequest to pass them
	// to the Buildbucket guts to be joined with the builder config. This is a
	// reverse of scheduleBuildRequestToStage(...). Note we completely ignore the
	// policy we returned in ValidateStage and just redo this calculation because
	// the builder config might have changed since then.
	scheduleBuildRequestFromPolicyTimeout(schReq, req.GetStage().
		GetExecutionPolicy().
		GetRequested().
		GetAttemptExecutionPolicyTemplate().
		GetTimeout(),
	)

	// Override schReq.Mask to get all the fields of the created builds.
	schReq.Mask = &pb.BuildMask{
		AllFields: true,
	}

	// Dedup by stage attempt id.
	// RequestID cannot contain "/", while the stage ID (as part of attemptID)
	// is a freeformed string, so hash it.
	schReq.RequestId = generateRequestID(attemptIDStr)

	blds, merr := scheduleBuilds(
		ctx,
		[]*pb.ScheduleBuildRequest{schReq},
		&scheduleBuildsParams{
			OverrideParent:         pBld,
			LaunchAsNative:         true,
			StageAttemptID:         attemptIDStr,
			StageAttemptToken:      req.GetStageAttemptToken(),
			TurboCIHost:            se.TurboCIHost,
			AllowInternalRequestID: true,
		})
	err = merr[0]
	if err != nil {
		if grpcutil.IsTransientCode(appstatus.Code(err)) {
			// Let TurboCI retry on transient errors.
			return nil, err
		}
		return &executorpb.RunStageResponse{}, cl.FailAttempt(ctx, &turboci.AttemptFailure{Err: err})
	}

	// Switch the attempt to SCHEDULED. If the attempt is already SCHEDULED or
	// RUNNING (either because RunStage was retried or the build started really
	// fast), this will just do nothing.
	cl.Build = blds[0]
	err = cl.SwitchAttemptToScheduled(ctx, buildToAttemptExecutionPolicy(blds[0]))
	if errors.Is(err, turboci.ErrAttemptAlreadyEnded) {
		err = abortBuildAsAbandoned(ctx, blds[0].Id)
	}
	return &executorpb.RunStageResponse{}, err
}

func generateRequestID(attemptID string) string {
	return fmt.Sprintf("%sturboci:%x", internalRequestIDPrefix, sha256.Sum256([]byte(attemptID)))
}
