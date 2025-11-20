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
	"slices"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/turboci/id"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
	orchestratorgrpcpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1/grpcpb"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

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
		return appstatus.ToError(status.Convert(err))
	}
	logging.Infof(ctx, "turbo-ci: workplan %s", id.ToString(plan.GetIdentifier()))

	// TODO: Call WriteNodes, then wait for the stage to start running.

	return appstatus.Errorf(codes.Unimplemented, "Turbo CI builds are not fully implemented yet")
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
