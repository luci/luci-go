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

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/botinfo"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

// DeleteBot implements the corresponding RPC method.
func (srv *BotsServer) DeleteBot(ctx context.Context, req *apipb.BotRequest) (*apipb.DeleteResponse, error) {
	if req.BotId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bot_id is required")
	}
	res := State(ctx).ACL.CheckBotPerm(ctx, req.BotId, acls.PermPoolsDeleteBot)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}
	update := &botinfo.Update{
		BotID:     req.BotId,
		EventType: model.BotEventDeleted,
	}
	info, err := update.Submit(ctx, func(botID, taskID string) botinfo.AbandonedTaskFinalizer {
		return &tasks.AbandonOp{
			BotID:          botID,
			TaskID:         taskID,
			LifecycleTasks: srv.TaskLifecycleTasks,
			ServerVersion:  srv.ServerVersion,
		}
	})
	switch {
	case err != nil:
		return nil, status.Errorf(codes.Internal, "datastore error deleting the bot")
	case info.BotInfo == nil:
		return nil, status.Errorf(codes.NotFound, "%q not found", req.BotId)
	default:
		return &apipb.DeleteResponse{Deleted: true}, nil
	}
}
