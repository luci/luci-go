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

	"google.golang.org/protobuf/types/known/emptypb"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
)

// GetDetails implements the corresponding RPC method.
func (srv *SwarmingServer) GetDetails(ctx context.Context, _ *emptypb.Empty) (*apipb.ServerDetails, error) {
	state := State(ctx)

	res := state.ACL.CheckServerPerm(ctx, acls.PermServersPeek)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	return &apipb.ServerDetails{
		ServerVersion:            srv.ServerVersion,
		BotVersion:               state.Config.VersionInfo.StableBot.Digest,
		DisplayServerUrlTemplate: state.Config.Settings().GetDisplayServerUrlTemplate(),
		CasViewerServer:          state.Config.Settings().GetCas().GetViewerServer(),
	}, nil
}
