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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// DeleteBot implements the corresponding RPC method.
func (*BotsServer) DeleteBot(ctx context.Context, req *apipb.BotRequest) (*apipb.DeleteResponse, error) {
	if req.BotId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bot_id is required")
	}
	res := State(ctx).ACL.CheckBotPerm(ctx, req.BotId, acls.PermPoolsDeleteBot)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	// See if the bot exists right now.
	info := &model.BotInfo{Key: model.BotInfoKey(ctx, req.BotId)}
	switch err := datastore.Get(ctx, info); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "%q not found", req.BotId)
	case err != nil:
		logging.Errorf(ctx, "Error fetching BotInfo for %q: %s", req.BotId, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching the bot")
	}

	// delete the bot.
	err := datastore.Delete(ctx, info)
	if err != nil {
		logging.Errorf(ctx, "Error deleting BotInfo for %q: %s", req.BotId, err)
		return nil, status.Errorf(codes.Internal, "datastore error deleting the bot")
	}
	return &apipb.DeleteResponse{Deleted: true}, nil
}
