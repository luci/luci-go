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
	"encoding/base64"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/gerritauth"
	"go.chromium.org/luci/turboci/id"
	"go.chromium.org/luci/turboci/rpc/write"
	"go.chromium.org/luci/turboci/value"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// rootBuildStageID is used as a stage ID of a root build Turbo CI stage.
const rootBuildStageID = "$init"

// checkTurboCIOAuthScope logs a warning if the call doesn't have credentials
// necessary to call Turbo CI APIs.
//
// See also turboCICallerCreds(...) where these credentials are actually
// converted into Turbo CI RPC credentials if the call ends up using Turbo CI.
func checkTurboCIOAuthScope(ctx context.Context) {
	// Acting as project is allowed, we'll use project-scoped accounts.
	if auth.CurrentIdentity(ctx).Kind() == identity.Project {
		logging.Infof(ctx, "turbo-ci: acting as project: %s", auth.CurrentIdentity(ctx))
		return
	}

	// If got an OAuth2 token, check it has the necessary scope.
	if info := auth.GetGoogleOAuth2Info(ctx); info != nil {
		if !slices.Contains(info.Scopes, scopes.AndroidBuild) {
			logging.Warningf(ctx, "turbo-ci: no OAuth scope: %s", auth.CurrentIdentity(ctx))
		} else {
			logging.Infof(ctx, "turbo-ci: good OAuth token: %s", auth.CurrentIdentity(ctx))
		}
		return
	}

	// JWT-based credentials are allowed (as impersonation tokens).
	if openid.GetOpenIDTokenInfo(ctx) != nil {
		logging.Infof(ctx, "turbo-ci: good OIDC JWT: %s", auth.CurrentIdentity(ctx))
		return
	}
	if gerritauth.GetAssertedInfo(ctx) != nil {
		logging.Infof(ctx, "turbo-ci: good Gerrit JWT: %s", auth.CurrentIdentity(ctx))
		return
	}

	logging.Warningf(ctx, "turbo-ci: unsupported credentials: %s", auth.CurrentIdentity(ctx))
}

// launchTurboCIRoot launches a root build via a new Turbo CI workplan.
//
// Updates `build` in-place. Returns appstatus errors.
func launchTurboCIRoot(ctx context.Context, req *pb.ScheduleBuildRequest, build *model.Build) error {
	logging.Infof(ctx, "turbo-ci: launching new workplan with %s", protoutil.FormatBuilderID(req.Builder))

	// We'll create the work plan and will poll the state of the added node as
	// Buildbucket itself (using a project-scoped account), but will do WriteNodes
	// as the caller. That way we can grant permission to create workplans only
	// to the Buildbucket, while still allowing normal callers add nodes to it
	// using Turbo CI API directly (making Buildbucket piggy back on it as well).
	projectCreds, err := turboci.ProjectRPCCredentials(ctx, req.Builder.Project)
	if err != nil {
		return appstatus.Errorf(codes.Internal, "project credentials: %s", err)
	}
	callerCreds, err := turboCICallerCreds(ctx)
	if err != nil {
		return err
	}

	// We need some idempotency key to create workplans. If the caller didn't
	// supply their own, generate some random one. Retries will not be idempotent,
	// but there's nothing we can do about that. Note that Turbo CI idempotency
	// key are already scoped to a concrete caller, so we don't need to prefix
	// them with the user ID. We need to make sure they are long enough though,
	// so hash them.
	idempotencyKey := req.RequestId
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	} else {
		digest := sha256.Sum256([]byte(idempotencyKey))
		idempotencyKey = base64.StdEncoding.EncodeToString(digest[:])
	}

	plan, err := turboci.CreateWorkPlan(ctx, orchestratorpb.CreateWorkPlanRequest_builder{
		Realm:          proto.String(build.Realm()),
		IdempotencyKey: proto.String(idempotencyKey),
	}.Build(), grpc.PerRPCCredentials(projectCreds))
	if err != nil {
		logging.Errorf(ctx, "turbo-ci: CreateWorkPlan call failed: %s", err)
		return appstatus.FromStatusErr(turboci.AdjustTurboCIRPCError(err))
	}
	logging.Infof(ctx, "turbo-ci: workplan %s", id.ToString(plan.GetIdentifier()))

	// The initial stage has a predefined ID.
	rootStageID := idspb.Stage_builder{
		WorkPlan:   plan.GetIdentifier(),
		Id:         proto.String(rootBuildStageID),
		IsWorknode: proto.Bool(false),
	}.Build()

	stages := []turboci.ScheduleBuildRequestStage{
		scheduleBuildRequestToStage(req, build, rootStageID),
	}

	// Submit the build stage as an end user (to attribute it to whoever triggers
	// the build). Use a well-known ID. If this is a retry, then this will
	// silently succeed without creating a duplicate.
	mErr := (&turboci.Client{
		Creds: callerCreds,
		Plan:  plan.GetIdentifier(),
		Token: plan.GetCreatorToken(),
	}).WriteStages(ctx, stages)
	err = mErr.First()
	if err != nil {
		logging.Errorf(ctx, "turbo-ci: failed to submit the stage: %s", err)
		return appstatus.FromStatusErr(err)
	}

	// Wait until the stage starts running and gets a build ID assigned to it.
	buildID, err := pollStage(ctx, &turboci.Client{
		Creds: projectCreds,
		Plan:  plan.GetIdentifier(),
		Token: plan.GetCreatorToken(),
	}, rootStageID)
	if err != nil {
		logging.Errorf(ctx, "turbo-ci: giving up: %s", err)
		return appstatus.FromStatusErr(err)
	}

	// Update `build` in-place with the state of the build right now.
	logging.Infof(ctx, "turbo-ci: got build ID %d", buildID)
	*build = model.Build{ID: buildID}
	if err := datastore.Get(ctx, build); err != nil {
		logging.Errorf(ctx, "turbo-ci: failed to get build %d: %s", buildID, err)
		return appstatus.Errorf(codes.Internal, "failed to get a build associated with the submitted stage: %s", err)
	}
	return nil
}

// launchTurboCIChildren launches child builds by adding them to an existing
// workplan.
//
// Updates `builds` in-place, returns exactly len(builds) appstatus errors.
func launchTurboCIChildren(ctx context.Context, parent *model.Build, reqs []*pb.ScheduleBuildRequest, builds []*model.Build) errors.MultiError {
	commonErr := func(err error) errors.MultiError {
		return slices.Repeat(errors.MultiError{err}, len(builds))
	}

	toAppstatusErrs := func(origErrs errors.MultiError) errors.MultiError {
		var errs errors.MultiError
		for _, err := range origErrs {
			errs = append(errs, appstatus.FromStatusErr(err))
		}
		return errs
	}

	if parent.StageAttemptID == "" {
		// Should never happen.
		return commonErr(appstatus.Errorf(codes.FailedPrecondition, "parent %d has no stage attempt ID", parent.ID))
	}
	aID, err := id.FromString(parent.StageAttemptID)
	if err != nil {
		return commonErr(appstatus.Errorf(codes.FailedPrecondition, "parent %d has invalid stage attempt ID %q: %s", parent.ID, parent.StageAttemptID, err))
	}
	plan := aID.GetStageAttempt().GetStage().GetWorkPlan()

	callerCreds, err := turboCICallerCreds(ctx)
	if err != nil {
		return commonErr(appstatus.Errorf(codes.Internal, "caller credentials: %s", err))
	}
	cl := &turboci.Client{
		Creds: callerCreds,
		Plan:  plan,
		Token: parent.StageAttemptToken,
	}

	stages := make([]turboci.ScheduleBuildRequestStage, len(builds))
	stageIDs := make([]*idspb.Stage, len(builds))
	for i, req := range reqs {
		stageID := idspb.Stage_builder{
			WorkPlan:   plan,
			Id:         proto.String(generateStageID(req.GetRequestId())),
			IsWorknode: proto.Bool(false),
		}.Build()
		stages[i] = scheduleBuildRequestToStage(req, builds[i], stageID)
		stageIDs[i] = stageID
	}

	mErr := cl.WriteStages(ctx, stages)
	if mErr.First() != nil {
		return toAppstatusErrs(mErr)
	}

	// Wait for the stages to run and get build ids back.
	//
	// Reuse the client with callerCreds to poll the stages. The call should have
	// permissions to both write and read the stages.
	//
	// Note that this is different from the root build scenario where we use
	// project scoped account to poll the submitted stage. The reason is that the
	// caller there doesn't necessary have permission to query Turbo CI workplan (
	// the caller is role/buildbucket.triggerer, it doesn't grant permission to
	// interact directly with Turbo CI). We do it this way, so that all existing
	// role/buildbucket.triggerer can automagically be able to submit
	// Turbo CI-enable builds.
	buildIDs, mErr := pollStages(ctx, cl, stageIDs)
	if mErr.First() != nil {
		return toAppstatusErrs(mErr)
	}

	// Update `builds` in-place with the state of the builds right now.
	for i, buildID := range buildIDs {
		builds[i].ID = buildID
	}
	if err := datastore.Get(ctx, builds); err != nil {
		merr, ok := err.(errors.MultiError)
		if !ok {
			return commonErr(appstatus.Errorf(codes.Internal, "failed to get a build associated with the submitted stage: %s", err))
		}
		mErr := make(errors.MultiError, len(builds))
		for i, err := range merr {
			if err == nil {
				continue
			}
			mErr[i] = appstatus.Errorf(codes.Internal, "failed to get a build associated with the submitted stage: %s", err)
		}
		return mErr
	}
	return nil
}

func generateStageID(reqID string) string {
	var bytes []byte

	if reqID != "" {
		h := sha256.Sum256([]byte(reqID))
		bytes = h[:16]
	} else {
		u := uuid.New()
		bytes = u[:]
	}

	return base64.StdEncoding.EncodeToString(bytes)
}

func scheduleBuildRequestToStage(req *pb.ScheduleBuildRequest, build *model.Build, stageID *idspb.Stage) turboci.ScheduleBuildRequestStage {
	// Field masks make no sense inside Turbo CI stage args.
	req.Mask = nil
	req.Fields = nil

	// Extract timeouts (if any) into Turbo CI stage attempt execution policy,
	// since we want them to be enforced and visible via Turbo CI, not just
	// Buildbucket.
	timeouts := scheduleBuildRequestToPolicyTimeout(req)
	req.SchedulingTimeout = nil
	req.ExecutionTimeout = nil
	req.GracePeriod = nil

	return turboci.ScheduleBuildRequestStage{
		ID:       stageID,
		Req:      req,
		Realm:    build.Realm(),
		Timeouts: timeouts,
	}
}

// turboCICallerCreds returns credentials to use to make Turbo CI calls in
// context of a calling user.
//
// If the call comes from a regular user that sent OAuth access token, just
// forward their credentials (verifying they are sufficient first).
//
// If the call comes from a regular user that sent an OpenID Identity token or
// a Gerrit JWT token, use Buildbucket's own credentials, but attach the end
// user token as "x-turboci-impersonation-token" metadata. The impersonator
// credential will be used for OnePlatform API billing/quotas, and the end-user
// token will be used to actually authorize the request.
//
// If the call comes from "project:XXX" identity, use the corresponding
// project-scoped service account (since Turbo CI is not aware of "project:XXX"
// identities). This account will be converted back into "project:XXX" identity
// in the RunStage implementation.
//
// Returns appstatus errors.
func turboCICallerCreds(ctx context.Context) (credentials.PerRPCCredentials, error) {
	caller := auth.CurrentIdentity(ctx)
	if caller.Kind() == identity.Project {
		creds, err := turboci.ProjectRPCCredentials(ctx, caller.Value())
		if err != nil {
			return nil, appstatus.Errorf(codes.Internal, "project credentials: %s", err)
		}
		return creds, nil
	}

	// TODO: We may need to add some hack to make sure the token we've got has
	// sufficient lifetime. We'll use it repeatedly, but we have no way of
	// refreshing it. If it expires midway, the user will see some confusing
	// errors. Maybe its worth to check the expiry in advance and refuse the call
	// if the token expires too soon (e.g. by returning some transient error to
	// trigger a retry with a fresher token).

	// If we've got an OAuth2 access token, check it has the necessary scope and
	// then just forward it to the Turbo CI.
	if info := auth.GetGoogleOAuth2Info(ctx); info != nil {
		if !slices.Contains(info.Scopes, scopes.AndroidBuild) {
			return nil, appstatus.Errorf(
				codes.Unauthenticated,
				"Calls that schedule builds that run via Turbo CI must use OAuth2 access tokens "+
					"that have at least %q and %q scopes, but the token used by this call has following scopes: %v. "+
					"Make sure you are using the most recent version of the Buildbucket client. "+
					"This is required because Buildbucket service calls Turbo CI APIs on behalf "+
					"of clients by forwarding their OAuth2 access token (b/454194663).",
				scopes.AndroidBuild, scopes.Email, info.Scopes,
			)
		}
		creds, err := auth.GetPerRPCCredentials(ctx, auth.AsCredentialsForwarder)
		if err != nil {
			return nil, appstatus.Errorf(codes.Internal, "error setting up credentials forwarding: %s", err)
		}
		return creds, nil
	}

	// Recognize authentication methods that use tokens that Turbo CI understands
	// as impersonation tokens.
	var method, token string
	if info := openid.GetOpenIDTokenInfo(ctx); info != nil {
		method = "OIDC"
		token = info.JWT
	} else if info := gerritauth.GetAssertedInfo(ctx); info != nil {
		method = "Gerrit"
		token = info.JWT
	} else {
		return nil, appstatus.Errorf(codes.Unauthenticated, "call credentials are not compatible with Turbo CI API")
	}

	impersonator, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(scopes.Email, scopes.AndroidBuild))
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "failed to get Buildbucket own credentials: %s", err)
	}

	return &impersonatingCreds{
		impersonator: impersonator,
		header:       fmt.Sprintf("%s %s", method, token),
	}, nil
}

// impersonatingCreds implements credentials.PerRPCCredentials.
type impersonatingCreds struct {
	impersonator credentials.PerRPCCredentials
	header       string
}

func (ic *impersonatingCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	md, err := ic.impersonator.GetRequestMetadata(ctx, uri...)
	if err != nil {
		return nil, err
	}
	md = maps.Clone(md)
	md["x-turboci-impersonation-token"] = ic.header
	return md, nil
}

func (ic *impersonatingCreds) RequireTransportSecurity() bool {
	return ic.impersonator.RequireTransportSecurity()
}

// pollingSchedule defines how long to sleep between polling attempts.
func pollingSchedule(ctx context.Context, attempt, errs int) time.Duration {
	var ms float64

	jitter := func(max float64) float64 { return max * mathrand.Float64(ctx) }

	switch {
	case errs > 0:
		// Do exponential backoff on errors.
		ms = min(5000.0, 5.0*math.Pow(2, float64(errs)))
		ms *= 1.2 - jitter(0.4)
	case attempt == 0:
		// Sleep longer before the first poll attempt, since this is when we expect
		// the orchestrator to be doing the actual work starting the stage. It is
		// expected to take at least 200 ms. This may need tuning.
		ms = 200.0 + jitter(100.0)
	case attempt < 20:
		// Poll relatively frequently first several seconds. We expect the stage to
		// appear any moment now.
		ms = 100.0 + jitter(100.0)
	default:
		// If still unsuccessful after 20 attempts, something is wrong. Slow down.
		ms = 1000.0 + jitter(250.0)
	}

	return time.Duration(ms * float64(time.Millisecond))
}

// pollStage periodically queries the stage until it gets assigned a build ID.
//
// Returns gRPC status errors.
func pollStage(ctx context.Context, cl *turboci.Client, stageID *idspb.Stage) (int64, error) {
	deadline := clock.Now(ctx).Add(30 * time.Second)
	ctx, cancel := clock.WithDeadline(ctx, deadline)
	defer cancel()

	// The last seen transient error.
	var lastTransientErr error

	// How many query attempts were made (successful or not).
	attempt := 0
	// The number of consecutive transient errors.
	errs := 0

	// giveUpErr is returned when we give up waiting for the stage.
	giveUpErr := func() error {
		if lastTransientErr != nil {
			return lastTransientErr
		}
		return status.Errorf(codes.DeadlineExceeded, "giving up waiting for the build after %d query attempts", attempt)
	}

	// contextErr is returned when the context expires.
	contextErr := func() error {
		err := ctx.Err()
		if errors.Is(err, context.DeadlineExceeded) {
			return giveUpErr()
		}
		return status.FromContextError(ctx.Err()).Err()
	}

	for {
		// Figure out how long to sleep before the poll attempt.
		sleep := pollingSchedule(ctx, attempt, errs)

		// Don't sleep uselessly just to abort due to the overall deadline. Try to
		// leave at least 150ms for the final attempt. If we don't have enough time
		// to complete the RPC (per the 150ms estimate), don't even try. The only
		// exception is if this is the first attempt ever (this can happen if
		// the RPC request was started with a very tight deadline). In this edge
		// case giving up without trying at least once is unexpected.
		wakeUp := min(sleep, clock.Until(ctx, deadline)-150*time.Millisecond)
		if wakeUp <= 0 {
			if attempt > 0 {
				return 0, giveUpErr()
			}
			wakeUp = 0
		}
		logging.Infof(ctx, "turbo-ci: QueryNodes attempt #%d, sleeping %dms", attempt+1, wakeUp.Milliseconds())
		clock.Sleep(ctx, wakeUp)
		if ctx.Err() != nil {
			return 0, contextErr()
		}

		// Poll the current state of the stage.
		attempt++
		details, err := cl.QueryStage(ctx, stageID)
		if err != nil {
			if ctx.Err() != nil {
				return 0, contextErr()
			}
			if code := status.Code(err); grpcutil.IsTransientCode(code) || code == codes.DeadlineExceeded {
				logging.Warningf(ctx, "turbo-ci: %s: transient error querying the stage: %s", id.ToString(cl.Plan), err)
				errs++
				lastTransientErr = err
				continue
			}
			logging.Errorf(ctx, "turbo-ci: %s: fatal error querying the stage: %s", id.ToString(cl.Plan), err)
			return 0, err
		}

		errs = 0
		lastTransientErr = nil

		if details != nil {
			switch v := details.Result.(type) {
			case *pb.BuildStageDetails_Id:
				return v.Id, nil
			case *pb.BuildStageDetails_Error:
				return 0, status.FromProto(v.Error).Err()
			}
		}
	}
}

// pollStages periodically queries the stages until all of them get assigned a build ID.
//
// Returns a slice of build IDs and a slice of gRPC status errors, both of the
// same length as stageIDs.
func pollStages(ctx context.Context, cl *turboci.Client, stageIDs []*idspb.Stage) ([]int64, errors.MultiError) {
	buildIDs := make([]int64, len(stageIDs))
	mErr := make(errors.MultiError, len(stageIDs))

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(64)
	baseTime := clock.Now(ctx)
	for i, stageID := range stageIDs {
		// Pre-calculate each task's minimal allowed start time in the outer loop
		// (staggered by i * 10ms) to avoid the initial QPS spike and avoid useless
		// sleeps on the queued ones.
		allowedStartTime := baseTime.Add(time.Duration(i) * 10 * time.Millisecond)
		eg.Go(func() error {
			if sleep := allowedStartTime.Sub(clock.Now(ctx)); sleep > 0 {
				clock.Sleep(ctx, sleep)
				if ctx.Err() != nil {
					// The context was canceled while we slept.
					mErr[i] = ctx.Err()
					return nil
				}
			}

			buildIDs[i], mErr[i] = pollStage(ctx, cl, stageID)
			return nil
		})
	}
	_ = eg.Wait() // pollStage errors are captured in mErr.

	return buildIDs, mErr
}

// updateStageAttemptToScheduled sets the current stage attempt (conveyed by
// cl.Token as StageAttemptToken) to SCHEDULED and reports the build details
// (e.g. build ID).
//
// Returns appstatus errors.
func updateStageAttemptToScheduled(ctx context.Context, cl *turboci.Client, bld *pb.Build) error {
	// This can happen if a previous RunTask created the build but failed to
	// update TurboCI, so TurboCI retries the RunTask. And after TurboCI sends
	// the retry, the build starts running.
	// In this case Buildbucket skips updating TurboCI.
	if bld.Status != pb.Status_SCHEDULED {
		logging.Infof(ctx, "Build %d has passed SCHEDULED status, skip updating TurboCI", bld.Id)
		return nil
	}

	writeReq := write.NewRequest()
	writeReq.SetReason("Buildbucket build scheduled")

	curWrite := writeReq.GetCurrentAttempt()

	details := tasks.PopulateBuildDetails(bld)
	curWrite.AddDetails(value.MustWrites(details)...)

	updatedPolicy := buildToStagetAttemptExecutionPolicy(bld)
	st := curWrite.GetStateTransition()
	st.SetScheduled(updatedPolicy)

	_, err := cl.WriteNodes(ctx, writeReq)
	return handleStageAttemptStatusConflict(ctx, bld, err)
}

// updateStageAttemptToRunning sets the StageAttempt to RUNNING and reports its
// process_uid and details.
//
// Returns appstatus errors.
func updateStageAttemptToRunning(ctx context.Context, cl *turboci.Client, bld *pb.Build, reqID string) error {
	writeReq := write.NewRequest()
	writeReq.SetReason("Buildbucket build started")

	curWrite := writeReq.GetCurrentAttempt()

	details := tasks.PopulateBuildDetails(bld)
	curWrite.AddDetails(value.MustWrites(details)...)

	st := curWrite.GetStateTransition()
	st.SetRunning(reqID, nil)

	_, err := cl.WriteNodes(ctx, writeReq)
	return handleStageAttemptStatusConflict(ctx, bld, err)
}
