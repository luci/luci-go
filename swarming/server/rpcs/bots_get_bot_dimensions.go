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

	"go.chromium.org/luci/common/logging"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
)

// GetBotDimensions implements the corresponding RPC method.
func (s *BotsServer) GetBotDimensions(ctx context.Context, req *apipb.BotsDimensionsRequest) (*apipb.BotsDimensions, error) {
	state := State(ctx)

	// Get cached aggregated bot dimensions across all pools.
	dims, err := s.BotsDimensionsCache.Get(ctx)
	if err != nil {
		logging.Errorf(ctx, "Error fetching bot dimensions: %s", err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching aggregated bot dimensions")
	}

	// If asked for dimensions in a concrete pool, verify the caller can see bots
	// there and then return dimensions from that single pool.
	if req.Pool != "" {
		res := state.ACL.CheckPoolPerm(ctx, req.Pool, acls.PermPoolsListBots)
		if !res.Permitted {
			return nil, res.ToGrpcErr()
		}
		return dims.DimensionsInPools([]string{req.Pool}), nil
	}

	// An optimization for callers that are allowed to see all bots on the server.
	// This is also the only way to see info about pools that were deleted from
	// configs (if there are any dead bots left in these pools).
	res := state.ACL.CheckServerPerm(ctx, acls.PermPoolsListBots)
	switch {
	case res.InternalError:
		return nil, res.ToGrpcErr()
	case res.Permitted:
		return dims.DimensionsGlobally(), nil
	}

	// Discover all pools visible per pools.cfg config and return a union of bot
	// dimensions there.
	visible, err := state.ACL.FilterPoolsByPerm(ctx, state.Config.Pools(), acls.PermPoolsListBots)
	if err != nil {
		return nil, err
	}
	return dims.DimensionsInPools(visible), nil
}
