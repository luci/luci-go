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
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

func TestDeleteBot(t *testing.T) {
	t.Parallel()

	state := NewMockedRequestState()

	// Bots that should be visible.
	aliveBotID := "alive-bot"
	notFoundBotID := "not-found-bot"
	state.Configs.MockBot(aliveBotID, "visible-pool")
	state.Configs.MockBot(notFoundBotID, "visible-pool")
	state.Configs.MockPool("visible-pool", "project:visible-realm")
	state.MockPerm("project:visible-realm", acls.PermPoolsDeleteBot)

	// A bot the caller has no permissions over.
	hiddenBotID := "hidden-bot"
	state.Configs.MockBot(hiddenBotID, "hidden-pool")
	state.Configs.MockPool("hidden-pool", "project:hidden-realm")

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	aliveBot := &model.BotInfo{
		Key:        model.BotInfoKey(ctx, aliveBotID),
		Dimensions: []string{"a:1", "b:2"},
		Composite: []model.BotStateEnum{
			model.BotStateNotInMaintenance,
			model.BotStateAlive,
			model.BotStateHealthy,
			model.BotStateBusy,
		},
		FirstSeen: TestTime,
		TaskName:  "task-name",
		BotCommon: model.BotCommon{
			State:           []byte(`{"state": "1"}`),
			ExternalIP:      "1.2.3.4",
			AuthenticatedAs: identity.Identity("bot:" + aliveBotID),
			Version:         "some-version",
			Quarantined:     false,
			Maintenance:     "maintenance msg",
			TaskID:          "task-id",
			LastSeen:        datastore.NewUnindexedOptional(TestTime.Add(1 * time.Hour)),
			IdleSince:       datastore.NewUnindexedOptional(TestTime.Add(2 * time.Hour)),
		},
	}
	_ = datastore.Put(ctx, aliveBot)

	call := func(botID string) (*apipb.DeleteResponse, error) {
		ctx := MockRequestState(ctx, state)
		return (&BotsServer{}).DeleteBot(ctx, &apipb.BotRequest{
			BotId: botID,
		})
	}

	ftt.Run("Bad bot ID", t, func(t *ftt.Test) {
		_, err := call("")
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("No permissions", t, func(t *ftt.Test) {
		_, err := call(hiddenBotID)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})

	ftt.Run("Bot not found", t, func(t *ftt.Test) {
		_, err := call(notFoundBotID)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
	})

	ftt.Run("OK", t, func(t *ftt.Test) {
		rps, err := call(aliveBotID)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rps, should.Match(&apipb.DeleteResponse{Deleted: true}))
		err = datastore.Get(ctx, aliveBot)
		assert.Loosely(t, err, should.ErrLikeError(datastore.ErrNoSuchEntity))
	})
}
