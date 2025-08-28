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
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cursor"
	"go.chromium.org/luci/swarming/server/cursor/cursorpb"
	"go.chromium.org/luci/swarming/server/legacyapi"
	"go.chromium.org/luci/swarming/server/model"
)

// ListBots implements the corresponding RPC method.
func (srv *BotsServer) ListBots(ctx context.Context, req *apipb.BotsRequest) (*apipb.BotInfoListResponse, error) {
	var err error
	if req.Limit, err = ValidateLimit(req.Limit); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid limit: %s", err)
	}

	var dscursor *cursorpb.BotsCursor
	if req.Cursor != "" {
		dscursor, err = cursor.Decode[cursorpb.BotsCursor](ctx, cursorpb.RequestKind_LIST_BOTS, req.Cursor)
		if err != nil {
			return nil, err
		}
	}

	dims, err := model.NewFilter(req.Dimensions, model.ValidateAsDimensions, true)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid dimensions: %s", err)
	}
	if err := CheckListingPerm(ctx, dims, acls.PermPoolsListBots); err != nil {
		return nil, err
	}

	q := model.BotInfoQuery().Limit(req.Limit)
	q = model.FilterBotsByState(q, model.StateFilter{
		Quarantined:   req.Quarantined,
		InMaintenance: req.InMaintenance,
		IsDead:        req.IsDead,
		IsBusy:        req.IsBusy,
	})
	// The results are always ordered by the key (derived from the bot ID). There
	// are also no duplicates. Thus we can use the bot ID itself as a cursor.
	if dscursor != nil {
		q = q.Gt("__key__", model.BotInfoKey(ctx, dscursor.LastBotId))
	}
	// Split the dimensions filter into simpler filters supported natively by
	// the datastore with existing indices. In most cases there will be only one.
	multi := model.FilterBotsByDimensions(q, srv.BotQuerySplitMode, dims)

	dscursor = nil
	out := &apipb.BotInfoListResponse{
		DeathTimeout: State(ctx).Config.Settings().BotDeathTimeoutSecs,
	}
	err = datastore.RunMulti(ctx, multi, func(bot *model.BotInfo, cb datastore.CursorCB) error {
		out.Items = append(out.Items, bot.ToProto())
		if len(out.Items) == int(req.Limit) {
			dscursor = &cursorpb.BotsCursor{LastBotId: out.Items[len(out.Items)-1].BotId}
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		logging.Errorf(ctx, "Error querying BotInfo: %s", err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching bots")
	}

	if dscursor != nil {
		out.Cursor, err = cursor.Encode(ctx, cursorpb.RequestKind_LIST_BOTS, dscursor)
		if err != nil {
			return nil, err
		}
	}

	out.Now = timestamppb.New(clock.Now(ctx))
	return out, nil
}

// LegacyListBotsRequest converts a legacy request to a gRPC request.
func LegacyListBotsRequest(req *http.Request) (*apipb.BotsRequest, error) {
	limit, err := legacyapi.Int(req, "limit")
	if err != nil {
		return nil, err
	}
	cursor, err := legacyapi.String(req, "cursor")
	if err != nil {
		return nil, err
	}
	dims, err := legacyapi.Dimensions(req, "dimensions")
	if err != nil {
		return nil, err
	}
	quarantined, err := legacyapi.NullableBool(req, "quarantined")
	if err != nil {
		return nil, err
	}
	maintenance, err := legacyapi.NullableBool(req, "in_maintenance")
	if err != nil {
		return nil, err
	}
	dead, err := legacyapi.NullableBool(req, "is_dead")
	if err != nil {
		return nil, err
	}
	busy, err := legacyapi.NullableBool(req, "is_busy")
	if err != nil {
		return nil, err
	}
	return &apipb.BotsRequest{
		Limit:         int32(limit),
		Cursor:        cursor,
		Dimensions:    dims,
		Quarantined:   quarantined,
		InMaintenance: maintenance,
		IsDead:        dead,
		IsBusy:        busy,
	}, nil
}

// LegacyListBotsResponse converts a gRPC response to a legacy response.
func LegacyListBotsResponse(res *apipb.BotInfoListResponse) (any, error) {
	type KV struct {
		Key   string   `json:"key"`
		Value []string `json:"value"`
	}

	type BotInfo struct {
		AuthenticatedAs string `json:"authenticated_as"`
		BotId           string `json:"bot_id"`
		Deleted         bool   `json:"deleted"`
		Dimensions      []KV   `json:"dimensions"`
		ExternalIp      string `json:"external_ip"`
		FirstSeenTs     string `json:"first_seen_ts"`
		IsDead          bool   `json:"is_dead"`
		LastSeenTs      string `json:"last_seen_ts"`
		MaintenanceMsg  string `json:"maintenance_msg"`
		Quarantined     bool   `json:"quarantined"`
		State           string `json:"state"`
		TaskId          string `json:"task_id"`
		TaskName        string `json:"task_name"`
		Version         string `json:"version"`
	}

	bots := make([]BotInfo, 0, len(res.Items))
	for _, bot := range res.Items {
		dims := make([]KV, 0, len(bot.Dimensions))
		for _, kv := range bot.Dimensions {
			dims = append(dims, KV{
				Key:   kv.Key,
				Value: kv.Value,
			})
		}
		bots = append(bots, BotInfo{
			AuthenticatedAs: bot.AuthenticatedAs,
			BotId:           bot.BotId,
			Deleted:         bot.Deleted,
			Dimensions:      dims,
			ExternalIp:      bot.ExternalIp,
			FirstSeenTs:     legacyapi.ToTime(bot.FirstSeenTs),
			IsDead:          bot.IsDead,
			LastSeenTs:      legacyapi.ToTime(bot.LastSeenTs),
			MaintenanceMsg:  bot.MaintenanceMsg,
			Quarantined:     bot.Quarantined,
			State:           bot.State,
			TaskId:          bot.TaskId,
			TaskName:        bot.TaskName,
			Version:         bot.Version,
		})
	}

	return struct {
		Cursor       string    `json:"cursor,omitempty"`
		DeathTimeout int32     `json:"death_timeout,string"`
		Items        []BotInfo `json:"items,omitempty"`
		Now          string    `json:"now"`
	}{
		res.Cursor,
		res.DeathTimeout,
		bots,
		legacyapi.ToTime(res.Now),
	}, nil
}
