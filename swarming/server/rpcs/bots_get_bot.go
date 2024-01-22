// Copyright 2023 The LUCI Authors.
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

// GetBot implements the corresponding RPC method.
func (*BotsServer) GetBot(ctx context.Context, req *apipb.BotRequest) (*apipb.BotInfo, error) {
	if req.BotId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bot_id is required")
	}
	res := State(ctx).ACL.CheckBotPerm(ctx, req.BotId, acls.PermPoolsListBots)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	// See if the bot exists right now.
	info := &model.BotInfo{Key: model.BotInfoKey(ctx, req.BotId)}
	switch err := datastore.Get(ctx, info); {
	case err == nil:
		return info.ToProto(), nil
	case !errors.Is(err, datastore.ErrNoSuchEntity):
		logging.Errorf(ctx, "Error fetching BotInfo for %q: %s", req.BotId, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching the bot")
	}

	// If there is no BotInfo, it means the bot doesn't currently exist (i.e. it
	// was deleted or has never existed). Look into the event history to get its
	// last known state if any. If the history is empty, it means the bot has
	// never existed or had been deleted long time ago. If there's a history, it
	// means the bot existed at some point, but has been deleted.
	var ev *model.BotEvent
	q := model.BotEventsQuery(ctx, req.BotId).Limit(1)
	err := datastore.Run(ctx, q, func(ent *model.BotEvent) error {
		ev = ent
		return datastore.Stop
	})
	switch {
	case err != nil:
		logging.Errorf(ctx, "Error querying BotEvent for %q: %s", req.BotId, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching the bot")
	case ev == nil:
		return nil, status.Errorf(codes.NotFound, "no such bot: %s", req.BotId)
	}

	// A bot may have connected right after datastore.Get(...) above, which
	// created a brand new BotEvent record that the datastore.Run(...) query
	// fetched. Here, another datastore.Get(...) is made to make sure the bot is
	// still actually missing. If we skip this check, we may accidentally mark
	// a very new bot as deleted.
	//
	// See https://crbug.com/1407381 for more information.
	switch err := datastore.Get(ctx, info); {
	case err == nil:
		return info.ToProto(), nil
	case !errors.Is(err, datastore.ErrNoSuchEntity):
		logging.Errorf(ctx, "Error re-fetching BotInfo for %q: %s", req.BotId, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching the bot")
	}

	// This bot existed (has a history), but is not connected anymore. Reconstruct
	// its BotInfo from the last historic entry.
	info.BotCommon = ev.BotCommon
	info.Dimensions = ev.Dimensions
	pb := info.ToProto()
	pb.Deleted = true
	return pb, nil
}
