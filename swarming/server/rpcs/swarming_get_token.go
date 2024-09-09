// Copyright 2024 The LUCI Authors.
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

package rpcs

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// GetToken implements the corresponding RPC method.
func (srv *SwarmingServer) GetToken(ctx context.Context, _ *emptypb.Empty) (*apipb.BootstrapToken, error) {
	res := State(ctx).ACL.CheckServerPerm(ctx, acls.PermPoolsCreateBot)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}
	tok, err := model.GenerateBootstrapToken(ctx, auth.CurrentIdentity(ctx))
	if err != nil {
		logging.Errorf(ctx, "Failed to generate bootstrap token: %s", err)
		return nil, status.Errorf(codes.Internal, "failed to generate the token")
	}
	return &apipb.BootstrapToken{BootstrapToken: tok}, nil
}
