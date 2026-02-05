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
	"fmt"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/turboci/data"
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/workplan"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// Client is a short-lived TurboCIOrchestratorClient scoped to a
// particular caller and plan.
type Client struct {
	// Creds is the credentials to use to call TurboCI Orchestrator.
	Creds credentials.PerRPCCredentials

	// Plan is the workplan ID the client works with.
	Plan *idspb.WorkPlan

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

// ProjectRPCCredentials creates a credential using the provided project.
func ProjectRPCCredentials(ctx context.Context, project string) (credentials.PerRPCCredentials, error) {
	if project == "" {
		return nil, appstatus.Errorf(codes.Unauthenticated, "no project specified")
	}

	// Regular users usually use BuildbucketScopeSet() to call us. Use the same
	// set of scopes when acting as a project account for consistency. It
	// includes the Turbo CI API scope.
	creds, err := auth.GetPerRPCCredentials(ctx,
		auth.AsProject,
		auth.WithProjectNoFallback(project),
		auth.WithScopes(scopes.BuildbucketScopeSet()...),
	)
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "error acting as project account of %q: %s", project, err)
	}
	return creds, nil
}

// stageID makes sure to populate `WorkPlan` field of `sid`.
func (c *Client) stageID(sid *idspb.Stage) *idspb.Stage {
	if sid.GetWorkPlan() == nil {
		sid = proto.CloneOf(sid)
		sid.SetWorkPlan(c.Plan)
	}
	return sid
}

// WriteStage submits the ScheduleBuildRequest stage under the given ID.
//
// This succeeds if the stage was submitted or it already exists.
func (c *Client) WriteStage(ctx context.Context, stageID *idspb.Stage, req *pb.ScheduleBuildRequest, realm string, timeouts *orchestratorpb.StageAttemptExecutionPolicy_Timeout) error {
	writeReq := write.NewRequest()
	writeReq.Msg.SetToken(c.Token)
	stg, err := writeReq.AddNewStage(c.stageID(stageID), req)
	if err != nil {
		return errors.Fmt("writeStage: NewStage: %w", err)
	}
	writeReq.AddReason("Submitting stage via Buildbucket")
	stg.Msg.SetRealm(realm)
	if timeouts != nil {
		stg.Msg.SetRequestedStageExecutionPolicy(orchestratorpb.StageExecutionPolicy_builder{
			AttemptExecutionPolicyTemplate: orchestratorpb.StageAttemptExecutionPolicy_builder{
				Timeout: timeouts,
			}.Build(),
		}.Build())
	}

	_, err = WriteNodes(ctx, writeReq.Msg, grpc.PerRPCCredentials(c.Creds))
	return AdjustTurboCIRPCError(err)
}

// buildStageDetailsTypeSet is a TypeSet composed of just BuildStageDetails.
var buildStageDetailsTypeSet = ((data.TypeSetBuilder{}).
	WithMessages((*pb.BuildStageDetails)(nil)).
	MustBuild())

// QueryStage queries the state of a submitted ScheduleBuildRequest stage.
//
// Returns (nil, nil) if the stage hasn't started yet. Returns BuildStageDetails
// if the stage was started (its build ID will be in BuildStageDetails.id) or it
// failed to start (in that case the error is set in BuildStageDetails.error).
//
// Returns an error only if the RPC itself fails.
func (c *Client) QueryStage(ctx context.Context, stageID *idspb.Stage) (*pb.BuildStageDetails, error) {
	// Make sure to use fully populated ID, since we serialize it in full to
	// get the map key below.
	stageID = c.stageID(stageID)

	queryReq := orchestratorpb.QueryNodesRequest_builder{
		Token: proto.String(c.Token),
		TypeInfo: orchestratorpb.TypeInfo_builder{
			Wanted: buildStageDetailsTypeSet,
		}.Build(),
		Query: []*orchestratorpb.Query{
			orchestratorpb.Query_builder{
				NodesById: orchestratorpb.Query_NodesByID_builder{
					Nodes: []*idspb.Identifier{
						id.Wrap(stageID),
					},
				}.Build(),
				SelectStages: &orchestratorpb.Query_SelectStages{},
				CollectStages: orchestratorpb.Query_CollectStages_builder{
					Attempts: orchestratorpb.CollectStageAttempts_COLLECT_STAGE_ATTEMPTS_LATEST.Enum(),
				}.Build(),
			}.Build(),
		},
	}.Build()

	resp, err := QueryNodes(ctx, queryReq, grpc.PerRPCCredentials(c.Creds))
	if err != nil {
		return nil, AdjustTurboCIRPCError(err)
	}
	stage := workplan.ToNodeBag(resp.GetWorkplans()...).Stage(stageID)
	if stage == nil {
		return nil, status.Errorf(codes.FailedPrecondition, "the stage is unexpectedly missing")
	}

	// This in theory should not be possible (because the stage we wrote doesn't
	// have any dependencies and can be attempted immediately), but check anyway.
	if len(stage.GetAttempts()) == 0 {
		logging.Warningf(ctx, "turbo-ci: %s: the stage has no attempts yet", id.ToString(stageID.GetWorkPlan()))
		return nil, nil
	}

	// Get the latest attempt. Note that when retrying a transiently failed
	// attempt, a new attempt is inserted transactionally when failing the
	// previous attempt. Thus the latest attempt is always either in one of
	// pending states, or it is executing, or it has already finished with
	// a fatal error.
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

// AttemptFailure contains the failure information to report to TurboCI when
// setting a stage attempt to INCOMPLETE.
type AttemptFailure struct {
	// Err is the human-readable error of a failure.
	Err error
	// Details is the machine-readable details of a failure, e.g.
	// ResourceExhaustion or Timeout.
	Details *pb.StatusDetails
}

// FailCurrentAttempt sets the current stage attempt (conveyed by the c.Token
// as StageAttemptToken) to INCOMPLETE and reports the failure.
func (c *Client) FailCurrentAttempt(ctx context.Context, attemptID *idspb.StageAttempt, failure *AttemptFailure, details ...proto.Message) error {
	logging.Errorf(ctx, "Fail stage attempt: %s", failure.Err)
	writeReq := write.NewRequest()
	writeReq.Msg.SetToken(c.Token)
	writeReq.AddReason(fmt.Sprintf("Set the stage attempt %s to INCOMPLETE due to an error: %s", id.ToString(attemptID), failure.Err))
	curWrite := writeReq.GetCurrentAttempt()
	prog, err := curWrite.AddProgress(failure.Err.Error(), failure.Details)
	if err != nil {
		return err
	}
	prog.SetIdempotencyKey("server/failure")

	if len(details) > 0 {
		curWrite.AddDetails(details...)
	}

	st := curWrite.GetStateTransition()
	st.SetIncomplete()

	_, err = WriteNodes(ctx, writeReq.Msg, grpc.PerRPCCredentials(c.Creds))
	return AdjustTurboCIRPCError(err)
}

// AbortCurrentAttempt sets the current stage attempt (by c.Token as StageAttemptToken)
// to INCOMPLETE and report the cancellation through progress.
func (c *Client) AbortCurrentAttempt(ctx context.Context, attemptID string, details ...proto.Message) error {
	logging.Infof(ctx, "Abort stage attempt: %s", attemptID)
	writeReq := write.NewRequest()
	writeReq.Msg.SetToken(c.Token)
	writeReq.AddReason("Buildbucket build cancelled")

	curWrite := writeReq.GetCurrentAttempt()
	curWrite.AddProgress("Cancelled via Buildbucket")

	if len(details) > 0 {
		curWrite.AddDetails(details...)
	}

	st := curWrite.GetStateTransition()
	st.SetIncomplete()

	_, nErr := WriteNodes(ctx, writeReq.Msg, grpc.PerRPCCredentials(c.Creds))
	return AdjustTurboCIRPCError(nErr)
}

// CompleteCurrentAttempt sets the current stage attempt (by c.Token as StageAttemptToken)
// to COMPLETE.
func (c *Client) CompleteCurrentAttempt(ctx context.Context, attemptID string) error {
	logging.Infof(ctx, "Complete stage attempt: %s", attemptID)
	writeReq := write.NewRequest()
	writeReq.Msg.SetToken(c.Token)
	writeReq.AddReason("Buildbucket build completed")
	st := writeReq.GetCurrentAttempt().GetStateTransition()
	st.SetComplete()
	_, nErr := WriteNodes(ctx, writeReq.Msg, grpc.PerRPCCredentials(c.Creds))
	return AdjustTurboCIRPCError(nErr)
}
