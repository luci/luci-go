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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// ListBots implements the corresponding RPC method.
func (srv *BotsServer) ListBots(ctx context.Context, req *apipb.BotsRequest) (*apipb.BotInfoListResponse, error) {
	var err error
	if req.Limit, err = ValidateLimit(req.Limit); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid limit: %s", err)
	}

	if req.Cursor != "" && !datastore.IsMultiCursorString(req.Cursor) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid cursor: wrong format")
	}

	dims, err := model.NewDimensionsFilter(req.Dimensions)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid dimensions: %s", err)
	}

	// If the query is restricted to some set of pools, need the permission in all
	// of them. Otherwise need the global server permissions, since an
	// unrestricted query can return bots from any pool.
	var res acls.CheckResult
	state := State(ctx)
	if pools := dims.Pools(); len(pools) != 0 {
		res = state.ACL.CheckAllPoolsPerm(ctx, pools, acls.PermPoolsListBots)
	} else {
		res = state.ACL.CheckServerPerm(ctx, acls.PermPoolsListBots)
	}
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	// Split the dimensions filter into simpler filters supported natively by
	// the datastore with existing indices. In most cases there will be only one.
	q := model.BotInfoQuery().Limit(req.Limit)
	q = model.FilterByState(q, model.StateFilter{
		Quarantined:   req.Quarantined,
		InMaintenance: req.InMaintenance,
		IsDead:        req.IsDead,
		IsBusy:        req.IsBusy,
	})
	multi := model.FilterByDimensions(q, srv.BotQuerySplitMode, dims)
	if req.Cursor != "" {
		multi, err = datastore.ApplyCursorString(ctx, multi, req.Cursor)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid cursor: %s", err)
		}
	}

	out := &apipb.BotInfoListResponse{
		DeathTimeout: state.Config.Settings().BotDeathTimeoutSecs,
	}
	err = datastore.RunMulti(ctx, multi, func(bot *model.BotInfo, cb datastore.CursorCB) error {
		out.Items = append(out.Items, bot.ToProto())
		if len(out.Items) == int(req.Limit) {
			cursor, err := cb()
			if err != nil {
				return err
			}
			out.Cursor = cursor.String()
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		logging.Errorf(ctx, "Error querying BotInfo: %s", err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching bots")
	}

	out.Now = timestamppb.New(clock.Now(ctx))
	return out, nil
}