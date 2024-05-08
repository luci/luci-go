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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestListBotEvents(t *testing.T) {
	t.Parallel()

	state := NewMockedRequestState()

	// A bot that should be visible.
	state.MockBot("visible-bot", "visible-pool")
	state.MockPool("visible-pool", "project:visible-realm")
	state.MockPerm("project:visible-realm", acls.PermPoolsListBots)

	// A bot the caller has no permissions over.
	state.MockBot("hidden-bot", "hidden-pool")
	state.MockPool("hidden-pool", "project:hidden-realm")

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	testTime := time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	total := 0
	putEvent := func(botID string, ts time.Duration) {
		total += 1
		err := datastore.Put(ctx, &model.BotEvent{
			Key:        datastore.NewKey(ctx, "BotEvent", "", int64(1000-total), model.BotRootKey(ctx, botID)),
			Timestamp:  testTime.Add(ts),
			EventType:  model.BotEventLog,
			Message:    fmt.Sprintf("at %s", ts),
			Dimensions: []string{"a:1", "b:2"},
			BotCommon: model.BotCommon{
				State:           []byte(`{"state": "1"}`),
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
			Ts:        timestamppb.New(testTime.Add(ts)),
			EventType: string(model.BotEventLog),
			Message:   fmt.Sprintf("at %s", ts),
			Dimensions: []*apipb.StringListPair{
				{Key: "a", Value: []string{"1"}},
				{Key: "b", Value: []string{"2"}},
			},
			State:           `{"state": "1"}`,
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

	Convey("Bad bot ID", t, func() {
		_, err := call(&apipb.BotEventsRequest{
			BotId: "",
			Limit: 10,
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("Limit is checked", t, func() {
		_, err := call(&apipb.BotEventsRequest{
			BotId: "doesnt-matter",
			Limit: -10,
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		_, err = call(&apipb.BotEventsRequest{
			BotId: "doesnt-matter",
			Limit: 1001,
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("Cursor is checked", t, func() {
		_, err := call(&apipb.BotEventsRequest{
			BotId:  "doesnt-matter",
			Cursor: "!!!!",
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("No permissions", t, func() {
		_, err := call(&apipb.BotEventsRequest{
			BotId: "hidden-bot",
			Limit: 10,
		})
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
	})

	Convey("Bot not in a config", t, func() {
		_, err := call(&apipb.BotEventsRequest{
			BotId: "unknown-bot",
			Limit: 10,
		})
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
	})

	Convey("All events", t, func() {
		resp, err := call(&apipb.BotEventsRequest{
			BotId: "visible-bot",
		})
		So(err, ShouldBeNil)
		So(resp.Cursor, ShouldEqual, "")
		So(resp.Items, ShouldResembleProto, []*apipb.BotEventResponse{
			expectedEvent("visible-bot", 4*time.Hour),
			expectedEvent("visible-bot", 3*time.Hour),
			expectedEvent("visible-bot", 2*time.Hour),
			expectedEvent("visible-bot", 1*time.Hour),
			expectedEvent("visible-bot", 0),
		})
	})

	Convey("Pagination", t, func() {
		resp, err := call(&apipb.BotEventsRequest{
			BotId: "visible-bot",
			Limit: 3,
		})
		So(err, ShouldBeNil)
		So(resp.Cursor, ShouldNotEqual, "")
		So(resp.Items, ShouldResembleProto, []*apipb.BotEventResponse{
			expectedEvent("visible-bot", 4*time.Hour),
			expectedEvent("visible-bot", 3*time.Hour),
			expectedEvent("visible-bot", 2*time.Hour),
		})
		resp, err = call(&apipb.BotEventsRequest{
			BotId:  "visible-bot",
			Limit:  3,
			Cursor: resp.Cursor,
		})
		So(err, ShouldBeNil)
		So(resp.Cursor, ShouldEqual, "")
		So(resp.Items, ShouldResembleProto, []*apipb.BotEventResponse{
			expectedEvent("visible-bot", 1*time.Hour),
			expectedEvent("visible-bot", 0),
		})
	})

	Convey("Time range", t, func() {
		resp, err := call(&apipb.BotEventsRequest{
			BotId: "visible-bot",
			Start: timestamppb.New(testTime.Add(time.Hour)),     // inclusive range
			End:   timestamppb.New(testTime.Add(4 * time.Hour)), // exclusive range
		})
		So(err, ShouldBeNil)
		So(resp.Cursor, ShouldEqual, "")
		So(resp.Items, ShouldResembleProto, []*apipb.BotEventResponse{
			expectedEvent("visible-bot", 3*time.Hour),
			expectedEvent("visible-bot", 2*time.Hour),
			expectedEvent("visible-bot", 1*time.Hour),
		})
	})
}
