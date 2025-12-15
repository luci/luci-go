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

package turboci

import (
	"context"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
	orchestratorgrpcpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1/grpcpb"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// Client is a short-lived TurboCIOrchestratorClient scoped to a
// particular caller and plan.
type Client struct {
	// Orch is the connection to the TurboCI Orchestrator to use.
	Orch orchestratorgrpcpb.TurboCIOrchestratorClient
	// Creads is the credential to use to call TurboCI Orchestrator.
	Creds credentials.PerRPCCredentials
	// PlanID is the workplan ID.
	PlanID string
	// Token is the token to use to call TurboCI Orchestrator.
	//
	// It could be a plan creator token or a stage attempt token.
	Token string
}

// AdjustTurboCIRPCError converts Unauthenticated error to Internal.
//
// As a precaution against Buildbucket clients treating it as a fatal build
// failure. See also the TODO in turboCICreds.
func AdjustTurboCIRPCError(err error) error {
	if status.Code(err) == codes.Unauthenticated {
		return status.Errorf(codes.Internal,
			"delegated call to the Turbo CI Orchestrator failed with an authentication error, "+
				"you may need to retry your Buildbucket request with a fresher access token; "+
				"the original error: %s", status.Convert(err).Message())
	}
	return err
}

// WriteStage submits the ScheduleBuildRequest stage under the given ID.
//
// This succeeds if the stage was submitted or it already exists.
func (c *Client) WriteStage(ctx context.Context, stageID string, req *pb.ScheduleBuildRequest, realm string, timeouts *orchestratorpb.StageAttemptExecutionPolicy_Timeout) error {
	sid, err := id.StageErr(id.StageNotWorknode, stageID)
	if err != nil {
		return errors.Fmt("writeStage: stageID: %w", err)
	}

	writeReq := write.NewRequest()
	writeReq.Msg.SetToken(c.Token)
	stg, err := writeReq.AddNewStage(sid, req)
	if err != nil {
		return errors.Fmt("writeStage: NewStage: %w", err)
	}
	stg.Msg.SetRealm(realm)
	if timeouts != nil {
		stg.Msg.SetRequestedStageExecutionPolicy(orchestratorpb.StageExecutionPolicy_builder{
			AttemptExecutionPolicyTemplate: orchestratorpb.StageAttemptExecutionPolicy_builder{
				Timeout: timeouts,
			}.Build(),
		}.Build())
	}
	_, err = c.Orch.WriteNodes(ctx, writeReq.Msg, grpc.PerRPCCredentials(c.Creds))
	return AdjustTurboCIRPCError(err)
}

// QueryStage queries the state of a submitted ScheduleBuildRequest stage.
//
// Returns (nil, nil) if the stage hasn't started yet. Returns BuildStageDetails
// if the stage was started (its build ID will be in BuildStageDetails.id) or it
// failed to start (in that case the error is set in BuildStageDetails.error).
//
// Returns an error only if the RPC itself fails.
func (c *Client) QueryStage(ctx context.Context, stageID string) (*pb.BuildStageDetails, error) {
	queryReq := orchestratorpb.QueryNodesRequest_builder{
		Token: proto.String(c.Token),
		TypeInfo: orchestratorpb.QueryNodesRequest_TypeInfo_builder{
			Wanted: []string{
				data.URL[*pb.BuildStageDetails](),
			},
		}.Build(),
		Query: []*orchestratorpb.Query{
			orchestratorpb.Query_builder{
				Select: orchestratorpb.Query_Select_builder{
					Nodes: []*idspb.Identifier{
						id.Wrap(id.Stage(stageID)),
					},
				}.Build(),
			}.Build(),
		},
	}.Build()

	resp, err := c.Orch.QueryNodes(ctx, queryReq, grpc.PerRPCCredentials(c.Creds))
	if err != nil {
		return nil, AdjustTurboCIRPCError(err)
	}
	stage := resp.GetGraph()[c.PlanID].GetStages()[stageID].GetStage()
	if stage == nil {
		return nil, status.Errorf(codes.FailedPrecondition, "the stage is unexpectedly missing")
	}

	// This in theory should not be possible (because the stage we wrote doesn't
	// have any dependencies and can be attempted immediately), but check anyway.
	if len(stage.GetAttempts()) == 0 {
		logging.Warningf(ctx, "turbo-ci: %s: the stage has no attempts yet", c.PlanID)
		return nil, nil
	}

	// Get the latest attempt. Note that when retrying a transiently failed
	// attempt, a new attempt is inserted transactionally when failing the
	// previous attempt. Thus the latest attempt is always either in one of
	// pending states, or it is execution, or it indicated a fatal error.
	attempt := stage.GetAttempts()[len(stage.GetAttempts())-1]
	details := data.FromMultipleValues[*pb.BuildStageDetails](attempt.GetDetails()...)
	if details != nil {
		return details, nil
	}

	// We have no BuildStageDetails: either we are still trying to launch the
	// stage or it was failed by the orchestrator without ever reaching the
	// Buildbucket stage executor (since the executor didn't write anything). If
	// it failed (i.e. the stage is not being attempted anymore), we synthesize
	// BuildStageDetails right here to indicate that.
	if stage.GetState() != orchestratorpb.StageState_STAGE_STATE_ATTEMPTING {
		return &pb.BuildStageDetails{
			Result: &pb.BuildStageDetails_Error{
				Error: &statuspb.Status{
					Code:    int32(codes.Aborted),
					Message: "the Turbo CI Orchestrator gave up trying to execute the build stage",
				},
			},
		}, nil
	}

	return nil, nil
}
