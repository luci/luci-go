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
	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

func TestDeleteBot(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
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
				State:           botstate.Dict{JSON: []byte(`{"state": "1"}`)},
				ExternalIP:      "1.2.3.4",
				AuthenticatedAs: identity.Identity("bot:" + aliveBotID),
				Version:         "some-version",
				Quarantined:     false,
				Maintenance:     "maintenance msg",
				TaskID:          "65aba3a3e6b99200",
				LastSeen:        datastore.NewUnindexedOptional(TestTime.Add(1 * time.Hour)),
				IdleSince:       datastore.NewUnindexedOptional(TestTime.Add(2 * time.Hour)),
			},
		}
		_ = datastore.Put(ctx, aliveBot)

		call := func(botID string, abandonedTaskID string) (*apipb.DeleteResponse, error) {

			var reqKey *datastore.Key
			var err error
			if abandonedTaskID != "" {
				reqKey, err = model.TaskIDToRequestKey(ctx, abandonedTaskID)
				assert.NoErr(t, err)
			}
			ctx := MockRequestState(ctx, state)
			srv := &BotsServer{
				TasksManager: &tasks.MockedManager{
					CompleteTxnMock: func(_ context.Context, op *tasks.CompleteOp) (*tasks.CompleteTxnOutcome, error) {
						assert.That(t, op.BotID, should.Equal(botID))
						assert.That(t, op.Request.Key, should.Match(reqKey))
						assert.That(t, op.Abandoned, should.BeTrue)
						return &tasks.CompleteTxnOutcome{}, nil
					},
				},
			}
			return srv.DeleteBot(ctx, &apipb.BotRequest{
				BotId: botID,
			})
		}

		t.Run("Bad bot ID", func(t *ftt.Test) {
			_, err := call("", "")
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("No permissions", func(t *ftt.Test) {
			_, err := call(hiddenBotID, "")
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("Bot not found", func(t *ftt.Test) {
			_, err := call(notFoundBotID, "")
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
		})

		t.Run("Task not found", func(t *ftt.Test) {
			rps, err := call(aliveBotID, "65aba3a3e6b99200")
			assert.NoErr(t, err)
			assert.Loosely(t, rps, should.Match(&apipb.DeleteResponse{Deleted: true}))
		})

		t.Run("OK", func(t *ftt.Test) {
			taskID := "65aba3a3e6b99200"
			reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
			assert.NoErr(t, err)
			tr := &model.TaskRequest{
				Key: reqKey,
			}
			assert.NoErr(t, datastore.Put(ctx, tr))

			rps, err := call(aliveBotID, taskID)
			assert.NoErr(t, err)
			assert.Loosely(t, rps, should.Match(&apipb.DeleteResponse{Deleted: true}))
			err = datastore.Get(ctx, aliveBot)
			assert.Loosely(t, err, should.ErrLikeError(datastore.ErrNoSuchEntity))
		})
	})

}
