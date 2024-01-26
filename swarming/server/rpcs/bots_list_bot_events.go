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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// ListBotEvents implements the corresponding RPC method.
func (*BotsServer) ListBotEvents(ctx context.Context, req *apipb.BotEventsRequest) (*apipb.BotEventsResponse, error) {
	if req.BotId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bot_id is required")
	}

	var err error
	if req.Limit, err = ValidateLimit(req.Limit); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid limit: %s", err)
	}

	var cursor datastore.Cursor
	if req.Cursor != "" {
		var err error
		cursor, err = datastore.DecodeCursor(ctx, req.Cursor)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid cursor: %s", err)
		}
	}

	res := State(ctx).ACL.CheckBotPerm(ctx, req.BotId, acls.PermPoolsListBots)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	q := model.BotEventsQuery(ctx, req.BotId)
	if req.Start != nil {
		q = q.Gte("ts", req.Start.AsTime())
	}
	if req.End != nil {
		q = q.Lt("ts", req.End.AsTime())
	}
	q = q.Limit(req.Limit)
	if cursor != nil {
		q = q.Start(cursor)
	}

	out := &apipb.BotEventsResponse{}

	err = datastore.Run(ctx, q, func(ev *model.BotEvent, cb datastore.CursorCB) error {
		out.Items = append(out.Items, ev.ToProto())
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
		logging.Errorf(ctx, "Error querying BotEvent for %q: %s", req.BotId, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching events")
	}

	out.Now = timestamppb.New(clock.Now(ctx))
	return out, nil
}
