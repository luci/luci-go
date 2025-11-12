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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	executorgrpcpb "go.chromium.org/turboci/proto/go/graph/executor/v1/grpcpb"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
	orchestratorgrpcpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1/grpcpb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// endUserMetadataKey is a gRPC metadata key that a trusted Turbo CI Stages
// backend is using to propagate the identity of an end user that inserted
// the stage.
const endUserMetadataKey = "x-turboci-end-user"

// turboCICallKey is used a context key for TurboCICall.
var turboCICallKey = "*TurboCICallInfo context key"

// TurboCICallInfo is propagated through Context in TurboCI Executor handlers.
//
// It holds per-request data that is common to all Executor RPC methods.
type TurboCICallInfo struct {
	// Stage is the stage being worked on.
	Stage *orchestratorpb.Stage
	// ScheduleBuild is deserialized from the stage args.
	ScheduleBuild *pb.ScheduleBuildRequest
}

// LogDetails logs some details about the call.
func (ci *TurboCICallInfo) LogDetails(ctx context.Context) {
	logging.Infof(ctx, "Stage ID: %s", ci.Stage.GetIdentifier())
	logging.Infof(ctx, "Submitted by: %s", auth.CurrentIdentity(ctx))
	logging.Infof(ctx, "ScheduleBuildRequest:\n%s", ci.ScheduleBuild)
}

// TurboCICall returns TurboCICallInfo stored in the context or nil.
func TurboCICall(ctx context.Context) *TurboCICallInfo {
	val, _ := ctx.Value(&turboCICallKey).(*TurboCICallInfo)
	return val
}

// TurboCIStageExecutor implements executorgrpcpb.TurboCIStageExecutorServer.
type TurboCIStageExecutor struct {
	executorgrpcpb.UnimplementedTurboCIStageExecutorServer

	// Orchestrator is the connection to the TurboCI Orchestrator to use.
	Orchestrator orchestratorgrpcpb.TurboCIOrchestratorClient
}

// TurboCIInterceptor is used for all TurboCI Stage Executor unary RPCs.
//
// It rejects calls that are not from turboCIStagesServiceAccount. It also
// modifies the auth state in the context such that the call is seen as coming
// from the actual end user that submitted the stage.
//
// Inserts TurboCICall into the context.
func TurboCIInterceptor(ctx context.Context, turboCIStagesServiceAccount string) grpc.UnaryServerInterceptor {
	expected, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, turboCIStagesServiceAccount))
	if err != nil {
		logging.Errorf(ctx, "malformed turbo-ci-stages service account: %q, all calls to TurboCI Stage executor will be rejected", turboCIStagesServiceAccount)
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		user := auth.CurrentIdentity(ctx)
		if user != expected {
			return nil, appstatus.Errorf(codes.PermissionDenied, "%q is not allowed to call TurboCI Stage Executor API", user)
		}

		// Grab the user that is submitting the stage from gRPC metadata. It is
		// placed there based on the user credentials by the trusted backend.
		endUserMD := metadata.ValueFromIncomingContext(ctx, endUserMetadataKey)
		if len(endUserMD) == 0 {
			return nil, appstatus.Errorf(codes.PermissionDenied, "missing %q metadata", endUserMetadataKey)
		}
		if len(endUserMD) > 1 {
			return nil, appstatus.Errorf(codes.PermissionDenied, "ambiguous %q metadata", endUserMetadataKey)
		}
		endUser, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, endUserMD[0]))
		if err != nil {
			return nil, appstatus.Errorf(codes.PermissionDenied, "unrecognized %q metadata email format: %s", endUserMetadataKey, err)
		}

		// This among other things verifies we've got ScheduleBuildRequest in
		// req.stage.args and deserializes it. We'll put it into the context to
		// avoid redoing this relatively expensive operation again in the actual
		// handler.
		call, err := extractTurboCICallInfo(req)
		if err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "bad request: %s", err)
		}
		ctx = context.WithValue(ctx, &turboCICallKey, call)

		// Extract the LUCI project the builder belongs to.
		if err := protoutil.ValidateBuilderID(call.ScheduleBuild.Builder); err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "bad builder ID: %s", err)
		}
		project := call.ScheduleBuild.Builder.Project

		// If this stage was submitted by a project-scoped account of the project
		// that owns the builder, treat it as "project:<...>" identity. That way
		// the request will successfully be authorized via role/luci.internal.system
		// role that "project:<...>" identities have in their corresponding
		// projects. Without this switch, we'd need to explicitly modify ACLs of all
		// LUCI projects.
		//
		// Note that if a LUCI project X submits (though another LUCI service)
		// a stage with a build in project Y, this identity switch won't engage
		// (because `call.submitter` has nothing to do with Y's project scoped
		// account). For such scenarios, X's project-scoped account will have to be
		// explicitly added to Y's ACL (just like we already add "project:X" account
		// into Y's ACL).
		//
		// We could theoretically keep a global mapping of project-scoped accounts
		// to LUCI projects they belong to, but this is relatively hard and seems
		// not worth supporting a rare scenario that has an easy workaround.
		data, err := auth.GetRealmData(ctx, realms.Join(project, realms.RootRealm))
		if err != nil {
			logging.Errorf(ctx, "Error looking up realm data for %q: %s", project, err)
			return nil, appstatus.Errorf(codes.Internal, "error looking up realm data")
		}
		if data.GetProjectScopedAccount() == endUser.Email() {
			endUser, err = identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.Project, project))
			if err != nil {
				return nil, appstatus.Errorf(codes.InvalidArgument, "unrecognized LUCI project name format: %s", err)
			}
		}

		// From now on the call will be treated as if it comes from the end user.
		ctx, err = auth.WithUncheckedImpersonation(ctx, endUser)
		if err != nil {
			logging.Errorf(ctx, "Failed to impersonate %q: %s", endUser, err)
			return nil, appstatus.Errorf(codes.Internal, "failed to impersonate the end user")
		}
		return handler(ctx, req)
	}
}

// extractTurboCICallInfo extracts TurboCICallInfo from an incoming Turbo CI
// Executor request.
//
// This deserializes a potentially big Any and thus is relatively expensive.
func extractTurboCICallInfo(req any) (*TurboCICallInfo, error) {
	var stage *orchestratorpb.Stage
	if knowsStage, ok := req.(interface{ GetStage() *orchestratorpb.Stage }); ok {
		stage = knowsStage.GetStage()
	} else {
		return nil, fmt.Errorf("no `stage` field in the request")
	}
	if stage == nil {
		return nil, fmt.Errorf("`stage` is not populated")
	}
	args := stage.GetArgs().GetValue()
	if args == nil {
		return nil, fmt.Errorf("`stage.args.value` is not populated")
	}
	var val pb.ScheduleBuildRequest
	if err := args.UnmarshalTo(&val); err != nil {
		return nil, fmt.Errorf("unexpected `stage.args.value` kind: %s", err)
	}
	return &TurboCICallInfo{
		Stage:         stage,
		ScheduleBuild: &val,
	}, nil
}
