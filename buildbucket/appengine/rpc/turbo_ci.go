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
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
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
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
	orchestratorgrpcpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1/grpcpb"

	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// rootBuildStageID is used as a stage ID of a root build Turbo CI stage.
const rootBuildStageID = "$init"

// checkTurboCIOAuthScope logs a warning if the call doesn't have an OAuth
// scope necessary to call Turbo CI APIs.
func checkTurboCIOAuthScope(ctx context.Context) {
	if auth.CurrentIdentity(ctx).Kind() == identity.Project {
		logging.Infof(ctx, "turbo-ci: acting as project: %s", auth.CurrentIdentity(ctx))
		return
	}
	switch info := auth.GetGoogleOAuth2Info(ctx); {
	case info == nil:
		logging.Warningf(ctx, "turbo-ci: no OAuth token: %s", auth.CurrentIdentity(ctx))
	case !slices.Contains(info.Scopes, scopes.AndroidBuild):
		logging.Warningf(ctx, "turbo-ci: no OAuth scope: %s", auth.CurrentIdentity(ctx))
	default:
		logging.Infof(ctx, "turbo-ci: good OAuth token: %s", auth.CurrentIdentity(ctx))
	}
}

// launchTurboCIRoot launches a root build via a new Turbo CI workplan.
//
// Updates `build` in-place. Returns appstatus errors.
func launchTurboCIRoot(ctx context.Context, req *pb.ScheduleBuildRequest, build *model.Build, orch orchestratorgrpcpb.TurboCIOrchestratorClient) error {
	logging.Infof(ctx, "turbo-ci: launching new workplan with %s", protoutil.FormatBuilderID(req.Builder))

	creds, err := turboCICreds(ctx)
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
	plan, err := orch.CreateWorkPlan(ctx, orchestratorpb.CreateWorkPlanRequest_builder{
		Realm:          proto.String(build.Realm()),
		IdempotencyKey: proto.String(idempotencyKey),
	}.Build(), grpc.PerRPCCredentials(creds))
	if err != nil {
		logging.Errorf(ctx, "turbo-ci: CreateWorkPlan call failed: %s", err)
		return appstatus.ToError(status.Convert(turboci.AdjustTurboCIRPCError(err)))
	}
	planID := plan.GetIdentifier().GetId()
	logging.Infof(ctx, "turbo-ci: workplan %s", planID)

	// The client configured to work with the created plan.
	client := &turboci.Client{
		Orch:   orch,
		Creds:  creds,
		PlanID: planID,
		Token:  plan.GetCreatorToken(),
	}

	// The mask field makes no sense inside Turbo CI stage args.
	req.Mask = nil

	// Extract timeouts (if any) into Turbo CI stage attempt execution policy,
	// since we want them to be enforced and visible via Turbo CI, not just
	// Buildbucket.
	timeouts := scheduleBuildRequestToPolicyTimeout(req)
	req.SchedulingTimeout = nil
	req.ExecutionTimeout = nil
	req.GracePeriod = nil

	// Submit the build as a stage under a well-known ID. If this is a retry,
	// then this will silently succeed without creating a duplicate.
	if err := client.WriteStage(ctx, rootBuildStageID, req, build.Realm(), timeouts); err != nil {
		logging.Errorf(ctx, "turbo-ci: failed to submit the stage: %s", err)
		return appstatus.ToError(status.Convert(err))
	}

	// Wait until the stage starts running and gets a build ID assigned to it.
	buildID, err := pollStage(ctx, client, rootBuildStageID)
	if err != nil {
		logging.Errorf(ctx, "turbo-ci: giving up: %s", err)
		return appstatus.ToError(status.Convert(err))
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
func launchTurboCIChildren(ctx context.Context, parent *model.Build, reqs []*pb.ScheduleBuildRequest, builds []*model.Build, orch orchestratorgrpcpb.TurboCIOrchestratorClient) errors.MultiError {
	return slices.Repeat(errors.MultiError{
		appstatus.Errorf(codes.Unimplemented, "Turbo CI child builds are not implemented yet"),
	}, len(reqs))
}

// turboCICreds returns credentials to use to make Turbo CI calls in context of
// a calling user.
//
// If the call comes from a regular user, just forward their credentials
// (verifying they are sufficient first).
//
// If the call comes from "project:XXX" identity, use the corresponding
// project-scoped service account (since we need a real OAuth token to call
// Turbo CI). This account will be converted back into "project:XXX" identity
// in the RunStage implementation.
//
// Returns appstatus errors.
func turboCICreds(ctx context.Context) (credentials.PerRPCCredentials, error) {
	caller := auth.CurrentIdentity(ctx)
	if caller.Kind() == identity.Project {
		// Regular users usually use BuildbucketScopeSet() to call us. Use the same
		// set of scopes when acting as a project account for consistency. It
		// includes the Turbo CI API scope.
		creds, err := auth.GetPerRPCCredentials(ctx,
			auth.AsProject,
			auth.WithProjectNoFallback(caller.Value()),
			auth.WithScopes(scopes.BuildbucketScopeSet()...),
		)
		if err != nil {
			return nil, appstatus.Errorf(codes.Internal, "error acting as project account of %q: %s", caller.Value(), err)
		}
		return creds, nil
	}

	// TODO: We may need to add some hack to make sure the token we've got has
	// sufficient lifetime. We'll use it repeatedly, but we have no way of
	// refreshing it. If it expires midway, the user will see some confusing
	// errors. Maybe its worth to check the expiry in advance and refuse the call
	// if the token expires too soon (e.g. by returning some transient error to
	// trigger a retry with a fresher token).

	// Verify we've got an OAuth2 access token we can forward to Turbo CI.
	switch info := auth.GetGoogleOAuth2Info(ctx); {
	case info == nil:
		return nil, appstatus.Errorf(
			codes.Unauthenticated,
			"Calls that schedule builds that run via Turbo CI must use OAuth2 access tokens "+
				"for authentication (OpenID Connect ID tokens and Gerrit ID tokens are not supported). "+
				"The OAuth2 access token must have at least %q and %q scopes. This is required because "+
				"Buildbucket service calls Turbo CI APIs on behalf of clients and Turbo CI APIs "+
				"require OAuth access tokens (b/454194663).",
			scopes.AndroidBuild, scopes.Email,
		)
	case !slices.Contains(info.Scopes, scopes.AndroidBuild):
		return nil, appstatus.Errorf(
			codes.Unauthenticated,
			"Calls that schedule builds that run via Turbo CI must use OAuth2 access tokens "+
				"that have at least %q and %q scopes, but the token used by this call has following scopes: %v. "+
				"Make sure you are using the most recent version of the Buildbucket client. "+
				"This is required because Buildbucket service calls Turbo CI APIs on behalf "+
				"of clients and Turbo CI APIs require OAuth access tokens (b/454194663).",
			scopes.AndroidBuild, scopes.Email, info.Scopes,
		)
	default:
		creds, err := auth.GetPerRPCCredentials(ctx, auth.AsCredentialsForwarder)
		if err != nil {
			return nil, appstatus.Errorf(codes.Internal, "error setting up credentials forwarding: %s", err)
		}
		return creds, nil
	}
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
func pollStage(ctx context.Context, cl *turboci.Client, stageID string) (int64, error) {
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
				logging.Warningf(ctx, "turbo-ci: %s: transient error querying the stage: %s", cl.PlanID, err)
				errs++
				lastTransientErr = err
				continue
			}
			logging.Errorf(ctx, "turbo-ci: %s: fatal error querying the stage: %s", cl.PlanID, err)
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
