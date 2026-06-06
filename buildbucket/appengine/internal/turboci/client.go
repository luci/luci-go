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
	"slices"

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
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc"
	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/value"
	"go.chromium.org/luci/turboci/workplan"
	stagepb "go.chromium.org/turboci/proto/go/data/stage/v1"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// ErrAttemptAlreadyEnded is returned by SwitchAttemptToScheduled and
// SwitchAttemptToRunning when trying to update an already ended attempt.
var ErrAttemptAlreadyEnded = appstatus.Errorf(codes.FailedPrecondition, "the stage attempt has already ended")

// Client is a short-lived TurboCIOrchestratorClient scoped to a
// particular caller, plan or stage attempt.
type Client struct {
	// Creds is the credentials to use to call TurboCI Orchestrator.
	Creds credentials.PerRPCCredentials

	// Plan is the workplan ID the client works with.
	Plan *idspb.WorkPlan

	// Token is the token to use to call TurboCI Orchestrator.
	//
	// It could be a plan creator token or a stage attempt token.
	Token string

	// Build is a build associated with the current stage attempt.
	//
	// Will be used to attach build details to WriteNodes requests that change
	// the attempt state.
	Build *pb.Build
}

// AdjustTurboCIRPCError converts Unauthenticated error to Internal.
//
// As a precaution against Buildbucket clients treating it as a fatal build
// failure. See also the TODO in turboCICreds.
//
// Recognizes and returns only gRPC status errors.
func AdjustTurboCIRPCError(err error) error {
	if status.Code(err) == codes.Unauthenticated {
		return status.Errorf(codes.Internal,
			"the delegated call to the Turbo CI Orchestrator failed with an authentication error, "+
				"you may need to retry your Buildbucket request with a fresher access token; "+
				"the original error: %s", status.Convert(err).Message())
	}
	return err
}

// ProjectRPCCredentials creates a credential using the provided project.
//
// Returned errors can be considered internal.
func ProjectRPCCredentials(ctx context.Context, project string) (credentials.PerRPCCredentials, error) {
	if project == "" {
		return nil, fmt.Errorf("no project specified")
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
		return nil, fmt.Errorf("acting as project account of %q: %w", project, err)
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

// ScheduleBuildRequestStage contains info to create a ScheduleBuildRequest stage.
type ScheduleBuildRequestStage struct {
	ID       *idspb.Stage
	Req      *pb.ScheduleBuildRequest
	Realm    string
	Timeouts *orchestratorpb.StageAttemptExecutionPolicy_Timeout
}

// WriteStages submits the ScheduleBuildRequest stages.
//
// This succeeds if the stages were submitted or already exist.
//
// Returns exactly len(stages) grpc errors, but currently they all have the same
// errors.
// TODO: b/512993859 - Extract per stage error from the structured details of
// the responded error.
func (c *Client) WriteStages(ctx context.Context, stages []ScheduleBuildRequestStage) errors.MultiError {
	if len(stages) == 0 {
		panic("WriteStages: no stages to submit")
	}
	writeReq := write.NewRequest()
	writeReq.SetReason("Submitting stage(s) via Buildbucket")
	for _, stage := range stages {
		stg := writeReq.AddNewStage(c.stageID(stage.ID), stage.Req)
		stg.Msg.SetRealm(stage.Realm)
		if stage.Timeouts != nil {
			stg.Msg.SetRequestedStageExecutionPolicy(orchestratorpb.StageExecutionPolicy_builder{
				AttemptExecutionPolicyTemplate: orchestratorpb.StageAttemptExecutionPolicy_builder{
					Timeout: stage.Timeouts,
				}.Build(),
			}.Build())
		}
	}
	writeReq.Msg.SetToken(c.Token)
	_, err := WriteNodes(ctx, writeReq.Msg, grpc.PerRPCCredentials(c.Creds))
	err = AdjustTurboCIRPCError(err)
	return convertToMultiError(err, len(stages))
}

func convertToMultiError(err error, n int) errors.MultiError {
	if err == nil {
		return nil
	}
	return slices.Repeat(errors.MultiError{err}, n)
}

// buildStageDetailsTypeSet is a TypeSet composed of just BuildStageDetails.
var buildStageDetailsTypeSet = ((value.TypeSetBuilder{}).
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
	bag := workplan.ToNodeBag(resp.GetValueData(), resp.GetWorkplans()...)
	stage := bag.Stage(stageID)
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
	details, err := value.Lookup[*pb.BuildStageDetails](bag.DataSource, attempt.GetDetails())
	if err != nil {
		return nil, err
	}
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
	// Err is the human-readable error of a failure (usually an appstatus error).
	Err error
	// Details is the machine-readable details of a failure, e.g.
	// ResourceExhaustion or Timeout.
	Details *pb.StatusDetails
}

// writeAttempt writes a change that switches the current attempt state.
//
// Attaches build details if they are available. Handles state conflict errors.
func (c *Client) writeAttempt(ctx context.Context, ignoreEnded bool, reason string, cb func(write.CurrentAttemptWrite)) error {
	logging.Infof(ctx, "Changing stage attempt state: %s", reason)

	req := write.NewRequest()
	req.Msg.SetToken(c.Token)
	req.SetReason(reason)

	cur := req.GetCurrentAttempt()
	if c.Build != nil {
		cur.AddDetails(value.MustWrites(buildDetails(c.Build))...)
	}
	cb(cur)

	resp, err := WriteNodes(ctx, req.Msg, grpc.PerRPCCredentials(c.Creds))
	if err == nil {
		logging.Infof(ctx, "WriteNodes succeeded: %v", resp)
		return nil
	}

	attempt, _ := rpc.StageAttemptCurrentState(err)
	if attempt == nil {
		logging.Errorf(ctx, "WriteNodes failed unexpectedly: %s", err)
		return appstatus.FromStatusErr(AdjustTurboCIRPCError(err))
	}
	logging.Infof(ctx, "WriteNodes conflict, current attempt state: %v", attempt)

	// We only really care about detecting unexpectedly finished attempts.
	// Conflicts involving non-final state (like when attempting to switch
	// RUNNING -> SCHEDULED) are expected and we can ignore them.
	ended := attempt.GetState() == orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_COMPLETE ||
		attempt.GetState() == orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_INCOMPLETE
	if !ended || ignoreEnded {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrAttemptAlreadyEnded, attempt.GetState())
}

// FailAttempt sets the current stage attempt to INCOMPLETE and reports
// the failure as a progress detail.
//
// If the attempt is already COMPLETE/INCOMPLETE, logs this and returns nil.
//
// Returns appstatus errors.
func (c *Client) FailAttempt(ctx context.Context, failure *AttemptFailure) error {
	return c.writeAttempt(ctx, true, fmt.Sprintf("Setting the stage attempt to INCOMPLETE: %s", failure.Err), func(cur write.CurrentAttemptWrite) {
		cur.GetStateTransition().SetIncomplete()
		prog := cur.AddProgress(failure.Err.Error(), value.MustWrite(failure.Details))
		prog.SetIdempotencyKey("server/failure")
	})
}

// AbortAttempt sets the current stage attempt to INCOMPLETE and reports the
// cancellation through progress.
//
// If the attempt is already COMPLETE/INCOMPLETE, logs this and returns nil.
//
// Returns appstatus errors.
func (c *Client) AbortAttempt(ctx context.Context) error {
	return c.writeAttempt(ctx, true, "Buildbucket build cancelled", func(cur write.CurrentAttemptWrite) {
		cur.GetStateTransition().SetIncomplete()
		cur.AddProgress("Cancelled via Buildbucket")
	})
}

// CompleteAttempt sets the current stage attempt to COMPLETE.
//
// If the attempt is already COMPLETE/INCOMPLETE, logs this and returns nil.
//
// Returns appstatus errors.
func (c *Client) CompleteAttempt(ctx context.Context) error {
	return c.writeAttempt(ctx, true, "Buildbucket build completed", func(cur write.CurrentAttemptWrite) {
		cur.GetStateTransition().SetComplete()
	})
}

// SwitchAttemptToScheduled switches the current stage attempt to SCHEDULED.
//
// If the attempt is already active, logs this and does nothing. If it is
// COMPLETE/INCOMPLETE returns an error wrapping ErrAttemptAlreadyEnded.
//
// Returns appstatus errors.
func (c *Client) SwitchAttemptToScheduled(ctx context.Context, policy *orchestratorpb.StageAttemptExecutionPolicy) error {
	return c.writeAttempt(ctx, false, "Buildbucket build scheduled", func(cur write.CurrentAttemptWrite) {
		cur.GetStateTransition().SetScheduled(policy)
	})
}

// SwitchAttemptToRunning switches the current stage attempt to RUNNING.
//
// If the attempt is already active, logs this and does nothing. If it is
// COMPLETE/INCOMPLETE returns an error wrapping ErrAttemptAlreadyEnded.
//
// Returns appstatus errors.
func (c *Client) SwitchAttemptToRunning(ctx context.Context, processUID string, policy *orchestratorpb.StageAttemptExecutionPolicy) error {
	return c.writeAttempt(ctx, false, "Buildbucket build started", func(cur write.CurrentAttemptWrite) {
		cur.GetStateTransition().SetRunning(processUID, policy)
	})
}

// SwitchAttemptToTearingDown switches the current stage attempt to
// TEARING_DOWN.
//
// If the attempt is already COMPLETE/INCOMPLETE, logs this and returns nil.
//
// Returns appstatus errors.
func (c *Client) SwitchAttemptToTearingDown(ctx context.Context, processUID string, policy *orchestratorpb.StageAttemptExecutionPolicy) error {
	return c.writeAttempt(ctx, true, "Buildbucket build tearing down", func(cur write.CurrentAttemptWrite) {
		cur.GetStateTransition().SetTearingDown()
	})
}

// buildDetails populates details about a build in a WriteNodes request.
//
// Most build stage attempt should have the details when they are advanced to
// SCHEDULED. But to ensure the builds have the details at all cases (e.g. the
// stage attempt could be advanced to RUNNING directly if that WriteNodes
// reaches TurboCI before the SCHEDULED one), Buildbucket adds the same details
// in all WriteNodes calls that update the attempt state.
func buildDetails(bld *pb.Build) []proto.Message {
	bldDetails := &pb.BuildStageDetails{
		Result: &pb.BuildStageDetails_Id{
			Id: bld.Id,
		},
	}
	if bld.GetInfra().GetBuildbucket().GetHostname() == "" {
		return []proto.Message{bldDetails}
	}
	commonDetails := stagepb.CommonStageAttemptDetails_builder{
		ViewUrls: map[string]*stagepb.CommonStageAttemptDetails_UrlDetails{
			"Buildbucket": stagepb.CommonStageAttemptDetails_UrlDetails_builder{
				Url: proto.String(fmt.Sprintf("https://%s/build/%d", bld.GetInfra().GetBuildbucket().GetHostname(), bld.Id)),
			}.Build(),
		},
	}.Build()
	return []proto.Message{bldDetails, commonDetails}
}
