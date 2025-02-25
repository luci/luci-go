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
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/secrets"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/model"
)

func TestListBotEvents(t *testing.T) {
	t.Parallel()

	state := NewMockedRequestState()

	// A bot that should be visible.
	state.Configs.MockBot("visible-bot", "visible-pool")
	state.Configs.MockPool("visible-pool", "project:visible-realm")
	state.MockPerm("project:visible-realm", acls.PermPoolsListBots)

	// A bot the caller has no permissions over.
	state.Configs.MockBot("hidden-bot", "hidden-pool")
	state.Configs.MockPool("hidden-pool", "project:hidden-realm")

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	total := 0
	putEvent := func(botID string, ts time.Duration) {
		total += 1
		err := datastore.Put(ctx, &model.BotEvent{
			Key:        datastore.NewKey(ctx, "BotEvent", "", int64(1000-total), model.BotRootKey(ctx, botID)),
			Timestamp:  TestTime.Add(ts),
			EventType:  model.BotEventLog,
			Message:    fmt.Sprintf("at %s", ts),
			Dimensions: []string{"a:1", "b:2"},
			BotCommon: model.BotCommon{
				State:           botstate.Dict{JSON: []byte(`{"state": "1"}`)},
				SessionID:       "test-session",
				ExternalIP:      "1.2.3.4",
				AuthenticatedAs: identity.Identity("bot:" + botID),
				Version:         "some-version",
				Quarantined:     false,
				Maintenance:     "maintenance msg",
				TaskID:          "task-id",
			},
		})
		if err != nil {
			panic(err)
		}
	}

	expectedEvent := func(botID string, ts time.Duration) *apipb.BotEventResponse {
		return &apipb.BotEventResponse{
			Ts:        timestamppb.New(TestTime.Add(ts)),
			EventType: string(model.BotEventLog),
			Message:   fmt.Sprintf("at %s", ts),
			Dimensions: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1"}},
				{Key: "b", Value: []string{"2"}},
			},
			State:           `{"state": "1"}`,
			SessionId:       "test-session",
			ExternalIp:      "1.2.3.4",
			AuthenticatedAs: "bot:" + botID,
			Version:         "some-version",
			Quarantined:     false,
			MaintenanceMsg:  "maintenance msg",
			TaskId:          "task-id",
		}
	}

	for i := 0; i < 5; i++ {
		putEvent("visible-bot", time.Hour*time.Duration(i))
	}
	putEvent("hidden-bot", 0)

	call := func(req *apipb.BotEventsRequest) (*apipb.BotEventsResponse, error) {
		ctx := MockRequestState(ctx, state)
		return (&BotsServer{}).ListBotEvents(ctx, req)
	}

	ftt.Run("Bad bot ID", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotEventsRequest{
			BotId: "",
			Limit: 10,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Limit is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotEventsRequest{
			BotId: "doesnt-matter",
			Limit: -10,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		_, err = call(&apipb.BotEventsRequest{
			BotId: "doesnt-matter",
			Limit: 1001,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Cursor is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotEventsRequest{
			BotId:  "doesnt-matter",
			Cursor: "!!!!",
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("No permissions", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotEventsRequest{
			BotId: "hidden-bot",
			Limit: 10,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})

	ftt.Run("Bot not in a config", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotEventsRequest{
			BotId: "unknown-bot",
			Limit: 10,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})

	ftt.Run("All events", t, func(t *ftt.Test) {
		resp, err := call(&apipb.BotEventsRequest{
			BotId: "visible-bot",
		})
		assert.NoErr(t, err)
		assert.Loosely(t, resp.Cursor, should.BeEmpty)
		assert.Loosely(t, resp.Items, should.Match([]*apipb.BotEventResponse{
			expectedEvent("visible-bot", 4*time.Hour),
			expectedEvent("visible-bot", 3*time.Hour),
			expectedEvent("visible-bot", 2*time.Hour),
			expectedEvent("visible-bot", 1*time.Hour),
			expectedEvent("visible-bot", 0),
		}))
	})

	ftt.Run("Pagination", t, func(t *ftt.Test) {
		resp, err := call(&apipb.BotEventsRequest{
			BotId: "visible-bot",
			Limit: 3,
		})
		assert.NoErr(t, err)
		assert.Loosely(t, resp.Cursor, should.NotEqual(""))
		assert.Loosely(t, resp.Items, should.Match([]*apipb.BotEventResponse{
			expectedEvent("visible-bot", 4*time.Hour),
			expectedEvent("visible-bot", 3*time.Hour),
			expectedEvent("visible-bot", 2*time.Hour),
		}))
		resp, err = call(&apipb.BotEventsRequest{
			BotId:  "visible-bot",
			Limit:  3,
			Cursor: resp.Cursor,
		})
		assert.NoErr(t, err)
		assert.Loosely(t, resp.Cursor, should.BeEmpty)
		assert.Loosely(t, resp.Items, should.Match([]*apipb.BotEventResponse{
			expectedEvent("visible-bot", 1*time.Hour),
			expectedEvent("visible-bot", 0),
		}))
	})

	ftt.Run("Time range", t, func(t *ftt.Test) {
		resp, err := call(&apipb.BotEventsRequest{
			BotId: "visible-bot",
			Start: timestamppb.New(TestTime.Add(time.Hour)),     // inclusive range
			End:   timestamppb.New(TestTime.Add(4 * time.Hour)), // exclusive range
		})
		assert.NoErr(t, err)
		assert.Loosely(t, resp.Cursor, should.BeEmpty)
		assert.Loosely(t, resp.Items, should.Match([]*apipb.BotEventResponse{
			expectedEvent("visible-bot", 3*time.Hour),
			expectedEvent("visible-bot", 2*time.Hour),
			expectedEvent("visible-bot", 1*time.Hour),
		}))
	})
}
