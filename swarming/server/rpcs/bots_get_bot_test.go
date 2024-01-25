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
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetBot(t *testing.T) {
	t.Parallel()

	state := NewMockedRequestState()

	// Bots that should be visible.
	state.MockBot("alive-bot", "visible-pool")
	state.MockBot("deleted-bot", "visible-pool")
	state.MockBot("unconnected-bot", "visible-pool")
	state.MockPool("visible-pool", "project:visible-realm")
	state.MockPerm("project:visible-realm", acls.PermPoolsListBots)

	// A bot the caller has no permissions over.
	state.MockBot("hidden-bot", "hidden-pool")
	state.MockPool("hidden-pool", "project:hidden-realm")

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	testTime := time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	fakeBotCommon := func(id string) model.BotCommon {
		return model.BotCommon{
			State:           []byte(`{"state": "1"}`),
			ExternalIP:      "1.2.3.4",
			AuthenticatedAs: identity.Identity("bot:" + id),
			Version:         "some-version",
			Quarantined:     false,
			Maintenance:     "maintenance msg",
			TaskID:          "task-id",
			LastSeen:        datastore.NewUnindexedOptional(testTime.Add(1 * time.Hour)),
			IdleSince:       datastore.NewUnindexedOptional(testTime.Add(2 * time.Hour)),
		}
	}

	_ = datastore.Put(ctx,
		&model.BotInfo{
			Key:        model.BotInfoKey(ctx, "alive-bot"),
			Dimensions: []string{"a:1", "b:2"},
			Composite: []model.BotStateEnum{
				model.BotStateNotInMaintenance,
				model.BotStateAlive,
				model.BotStateHealthy,
				model.BotStateBusy,
			},
			FirstSeen: testTime,
			TaskName:  "task-name",
			BotCommon: fakeBotCommon("alive-bot"),
		},
		&model.BotEvent{
			Key:        datastore.NewKey(ctx, "BotEvent", "", 200, model.BotRootKey(ctx, "deleted-bot")),
			Timestamp:  testTime, // old event, should be ignored
			Dimensions: []string{"ignore:me"},
			BotCommon:  fakeBotCommon("deleted-bot"),
		},
		&model.BotEvent{
			Key:        datastore.NewKey(ctx, "BotEvent", "", 100, model.BotRootKey(ctx, "deleted-bot")),
			Timestamp:  testTime.Add(time.Hour), // the most recent event, should be used
			Dimensions: []string{"use:me"},
			BotCommon:  fakeBotCommon("deleted-bot"),
		},
	)

	call := func(botID string) (*apipb.BotInfo, error) {
		ctx := MockRequestState(ctx, state)
		return (&BotsServer{}).GetBot(ctx, &apipb.BotRequest{
			BotId: botID,
		})
	}

	Convey("Bad bot ID", t, func() {
		_, err := call("")
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("No permissions", t, func() {
		_, err := call("hidden-bot")
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
	})

	Convey("Bot not in a config", t, func() {
		_, err := call("unknown-bot")
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
	})

	Convey("Unconnected bot", t, func() {
		_, err := call("unconnected-bot")
		So(err, ShouldHaveGRPCStatus, codes.NotFound)
	})

	Convey("Alive bot", t, func() {
		resp, err := call("alive-bot")
		So(err, ShouldBeNil)
		So(resp, ShouldResembleProto, &apipb.BotInfo{
			BotId:           "alive-bot",
			TaskId:          "task-id",
			TaskName:        "task-name",
			ExternalIp:      "1.2.3.4",
			AuthenticatedAs: "bot:alive-bot",
			FirstSeenTs:     timestamppb.New(testTime),
			LastSeenTs:      timestamppb.New(testTime.Add(1 * time.Hour)),
			MaintenanceMsg:  "maintenance msg",
			Dimensions: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1"}},
				{Key: "b", Value: []string{"2"}},
			},
			Version: "some-version",
			State:   `{"state": "1"}`,
		})
	})

	Convey("Deleted bot", t, func() {
		resp, err := call("deleted-bot")
		So(err, ShouldBeNil)
		So(resp, ShouldResembleProto, &apipb.BotInfo{
			BotId:           "deleted-bot",
			TaskId:          "task-id",
			ExternalIp:      "1.2.3.4",
			AuthenticatedAs: "bot:deleted-bot",
			LastSeenTs:      timestamppb.New(testTime.Add(1 * time.Hour)),
			MaintenanceMsg:  "maintenance msg",
			Dimensions: []*apipb.StringListPair{
				{Key: "use", Value: []string{"me"}},
			},
			Version: "some-version",
			State:   `{"state": "1"}`,
			Deleted: true,
		})
	})
}
