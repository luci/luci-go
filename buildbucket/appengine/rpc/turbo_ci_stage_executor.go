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
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	executorgrpcpb "go.chromium.org/turboci/proto/go/graph/executor/v1/grpcpb"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// testAllowFakeTestStage enables extractCallInfo recognize test-only args.
var testAllowFakeTestStage bool

// TurboCIStageExecutor implements executorgrpcpb.TurboCIStageExecutorServer.
type TurboCIStageExecutor struct {
	executorgrpcpb.UnimplementedTurboCIStageExecutorServer
}

// TurboCIInterceptor is used for all TurboCI Stage Executor unary RPCs.
//
// It rejects calls that are not from turboCIStagesServiceAccount. It also
// modifies the auth state in the context such that the call is seen as coming
// from the actual end user that submitted the stage.
func TurboCIInterceptor(ctx context.Context, turboCIStagesServiceAccount string) grpc.UnaryServerInterceptor {
	expected, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, turboCIStagesServiceAccount))
	if err != nil {
		logging.Errorf(ctx, "malformed turbo-ci-stages service account: %q, all calls to TurboCI Stage executor will be rejected", turboCIStagesServiceAccount)
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		user := auth.CurrentIdentity(ctx)
		if user != expected {
			return nil, status.Errorf(codes.PermissionDenied, "%q is not allowed to call TurboCI Stage Executor API", user)
		}

		call, err := extractCallInfo(req)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad request: %s", err)
		}

		// We'll treat the call as coming this user.
		endUser, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, call.submitter))
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "unrecognized submitter email format: %s", err)
		}

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
		data, err := auth.GetRealmData(ctx, realms.Join(call.project, realms.RootRealm))
		if err != nil {
			logging.Errorf(ctx, "Error looking up realm data for %q: %s", call.project, err)
			return nil, status.Errorf(codes.Internal, "error looking up realm data")
		}
		if data.GetProjectScopedAccount() == call.submitter {
			endUser, err = identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.Project, call.project))
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "unrecognized LUCI project name format: %s", err)
			}
		}

		// From now on the call will be treated as if it comes from the end user.
		ctx, err = auth.WithUncheckedImpersonation(ctx, endUser)
		if err != nil {
			logging.Errorf(ctx, "Failed to impersonate %q: %s", endUser, err)
			return nil, status.Errorf(codes.Internal, "failed to impersonate the end user")
		}
		return handler(ctx, req)
	}
}

// callInfo is extracted from incoming Turbo CI Executor requests.
type callInfo struct {
	// Submitter is an email of an end-user that submitted the stage to Turbo CI.
	submitter string
	// Project is a LUCI project that owns the builder being triggered.
	project string
}

// extractCallInfo extracts callInfo from incoming Turbo CI Executor requests.
func extractCallInfo(req any) (*callInfo, error) {
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

	if testAllowFakeTestStage {
		var val structpb.Value
		if args.UnmarshalTo(&val) == nil {
			asMap, _ := val.AsInterface().(map[string]any)
			if asMap == nil {
				return nil, fmt.Errorf("bad fake test stage args, not a map: %v", &val)
			}
			return &callInfo{
				submitter: asMap["submitter"].(string),
				project:   asMap["project"].(string),
			}, nil
		}
	}

	// TODO: Figure out how exactly this information is passed for real BB stages.
	return nil, fmt.Errorf("not implemented yet")
}
