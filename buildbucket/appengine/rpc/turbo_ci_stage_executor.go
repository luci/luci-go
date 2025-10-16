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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	executorgrpcpb "go.chromium.org/turboci/proto/go/graph/executor/v1/grpcpb"
)

// TurboCIStageExecutor implements executorgrpcpb.TurboCIStageExecutorServer.
type TurboCIStageExecutor struct {
	executorgrpcpb.UnimplementedTurboCIStageExecutorServer
}

// TurboCIInterceptor is used for all TurboCI Stage Executor unary RPCs.
//
// It rejects calls that are not from turboCIStagesServiceAccount.
func TurboCIInterceptor(ctx context.Context, turboCIStagesServiceAccount string) grpc.UnaryServerInterceptor {
	expected, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, turboCIStagesServiceAccount))
	if err != nil {
		logging.Errorf(ctx, "malformed turbo-ci-stages service account: %q, all calls to TurboCI Stage executor will be rejected", turboCIStagesServiceAccount)
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		user := auth.CurrentIdentity(ctx)
		if user != expected {
			return nil, status.Errorf(codes.PermissionDenied, "%q is not allowed to access the turboci stage executor", user)
		}
		return handler(ctx, req)
	}
}
