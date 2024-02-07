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
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// setupTestBots mocks a bunch of bots and pools.
func setupTestBots(ctx context.Context) *MockedRequestState {
	state := NewMockedRequestState()
	state.Configs.Settings.BotDeathTimeoutSecs = 1234

	state.MockPool("visible-pool1", "project:visible-realm")
	state.MockPool("visible-pool2", "project:visible-realm")
	state.MockPool("hidden-pool1", "project:hidden-realm")
	state.MockPool("hidden-pool2", "project:hidden-realm")

	state.MockPerm("project:visible-realm", acls.PermPoolsListBots)

	type testBot struct {
		id          string
		pool        string
		dims        []string
		quarantined bool
		maintenance bool
		busy        bool
		dead        bool
	}

	testBots := []testBot{}
	addMany := func(num int, pfx testBot) {
		id := pfx.id
		for i := 0; i < num; i++ {
			pfx.id = fmt.Sprintf("%s-%d", id, i)
			pfx.dims = []string{fmt.Sprintf("idx:%d", i), fmt.Sprintf("dup:%d", i)}
			testBots = append(testBots, pfx)
		}
	}

	addMany(3, testBot{
		id:   "visible1",
		pool: "visible-pool1",
	})
	addMany(3, testBot{
		id:   "visible2",
		pool: "visible-pool2",
	})
	addMany(3, testBot{
		id:          "quarantined",
		pool:        "visible-pool1",
		quarantined: true,
	})
	addMany(3, testBot{
		id:          "maintenance",
		pool:        "visible-pool1",
		maintenance: true,
	})
	addMany(3, testBot{
		id:   "busy",
		pool: "visible-pool1",
		busy: true,
	})
	addMany(3, testBot{
		id:   "dead",
		pool: "visible-pool1",
		dead: true,
	})
	addMany(3, testBot{
		id:   "hidden1",
		pool: "hidden-pool1",
	})
	addMany(3, testBot{
		id:   "hidden2",
		pool: "hidden-pool2",
	})

	for _, bot := range testBots {
		pick := func(attr bool, yes model.BotStateEnum, no model.BotStateEnum) model.BotStateEnum {
			if attr {
				return yes
			}
			return no
		}
		state.MockBot(bot.id, bot.pool) // add it to ACLs
		err := datastore.Put(ctx, &model.BotInfo{
			Key:        model.BotInfoKey(ctx, bot.id),
			Dimensions: append(bot.dims, "pool:"+bot.pool),
			Composite: []model.BotStateEnum{
				pick(bot.maintenance, model.BotStateInMaintenance, model.BotStateNotInMaintenance),
				pick(bot.dead, model.BotStateDead, model.BotStateAlive),
				pick(bot.quarantined, model.BotStateQuarantined, model.BotStateHealthy),
				pick(bot.busy, model.BotStateBusy, model.BotStateIdle),
			},
		})
		if err != nil {
			panic(err)
		}
	}

	return state
}

func TestListBots(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	state := setupTestBots(ctx)

	callImpl := func(ctx context.Context, req *apipb.BotsRequest) (*apipb.BotInfoListResponse, error) {
		return (&BotsServer{
			// memory.Use(...) datastore fake doesn't support IN queries currently.
			BotQuerySplitMode: model.SplitCompletely,
		}).ListBots(ctx, req)
	}
	call := func(req *apipb.BotsRequest) (*apipb.BotInfoListResponse, error) {
		return callImpl(MockRequestState(ctx, state), req)
	}
	callAsAdmin := func(req *apipb.BotsRequest) (*apipb.BotInfoListResponse, error) {
		return callImpl(MockRequestState(ctx, state.SetCaller(AdminFakeCaller)), req)
	}

	botIDs := func(bots []*apipb.BotInfo) []string {
		var ids []string
		for _, bot := range bots {
			ids = append(ids, bot.BotId)
		}
		return ids
	}

	Convey("Limit is checked", t, func() {
		_, err := call(&apipb.BotsRequest{
			Limit: -10,
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		_, err = call(&apipb.BotsRequest{
			Limit: 1001,
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("Cursor is checked", t, func() {
		_, err := call(&apipb.BotsRequest{
			Cursor: "!!!!",
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("Dimensions filter is checked", t, func() {
		_, err := call(&apipb.BotsRequest{
			Dimensions: []*apipb.StringPair{
				{Key: "", Value: ""},
			},
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("ACLs", t, func() {
		Convey("Listing only visible pools: OK", func() {
			_, err := call(&apipb.BotsRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|visible-pool2"},
				},
			})
			So(err, ShouldBeNil)
		})

		Convey("Listing visible and invisible pool: permission denied", func() {
			_, err := call(&apipb.BotsRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|hidden-pool1"},
				},
			})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		})

		Convey("Listing visible and invisible pool as admin: OK", func() {
			_, err := callAsAdmin(&apipb.BotsRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|hidden-pool1"},
				},
			})
			So(err, ShouldBeNil)
		})

		Convey("Listing all pools as non-admin: permission denied", func() {
			_, err := call(&apipb.BotsRequest{})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		})

		Convey("Listing all pools as admin: OK", func() {
			_, err := callAsAdmin(&apipb.BotsRequest{})
			So(err, ShouldBeNil)
		})
	})

	Convey("Filtering and cursors", t, func() {
		checkQuery := func(req *apipb.BotsRequest, expected []string) {
			// An unlimited query first.
			resp, err := callAsAdmin(req)
			So(err, ShouldBeNil)
			So(botIDs(resp.Items), ShouldResemble, expected)

			// A paginated one should return the same results.
			var out []*apipb.BotInfo
			var cursor string
			for {
				req.Cursor = cursor
				req.Limit = 2
				resp, err := callAsAdmin(req)
				So(err, ShouldBeNil)
				So(len(resp.Items), ShouldBeLessThanOrEqualTo, req.Limit)
				out = append(out, resp.Items...)
				cursor = resp.Cursor
				if cursor == "" {
					break
				}
			}
			So(botIDs(out), ShouldResemble, expected)
		}

		Convey("No filters", func() {
			checkQuery(
				&apipb.BotsRequest{},
				[]string{
					"busy-0", "busy-1", "busy-2",
					"dead-0", "dead-1", "dead-2",
					"hidden1-0", "hidden1-1", "hidden1-2",
					"hidden2-0", "hidden2-1", "hidden2-2",
					"maintenance-0", "maintenance-1", "maintenance-2",
					"quarantined-0", "quarantined-1", "quarantined-2",
					"visible1-0", "visible1-1", "visible1-2",
					"visible2-0", "visible2-1", "visible2-2",
				},
			)
		})

		Convey("Busy only", func() {
			checkQuery(
				&apipb.BotsRequest{
					IsBusy: apipb.NullableBool_TRUE,
				},
				[]string{
					"busy-0", "busy-1", "busy-2",
				},
			)
		})

		Convey("Dead only", func() {
			checkQuery(
				&apipb.BotsRequest{
					IsDead: apipb.NullableBool_TRUE,
				},
				[]string{
					"dead-0", "dead-1", "dead-2",
				},
			)
		})

		Convey("Maintenance only", func() {
			checkQuery(
				&apipb.BotsRequest{
					InMaintenance: apipb.NullableBool_TRUE,
				},
				[]string{
					"maintenance-0", "maintenance-1", "maintenance-2",
				},
			)
		})

		Convey("Quarantined only", func() {
			checkQuery(
				&apipb.BotsRequest{
					Quarantined: apipb.NullableBool_TRUE,
				},
				[]string{
					"quarantined-0", "quarantined-1", "quarantined-2",
				},
			)
		})

		// Note: assuming all "positive" filter checks passed, it is sufficient to
		// test only one "negative" filter. Negative tests are just a tweak in
		// a code path already tested by "positive" filters.
		Convey("Non-busy only", func() {
			checkQuery(
				&apipb.BotsRequest{
					IsBusy: apipb.NullableBool_FALSE,
				},
				[]string{
					"dead-0", "dead-1", "dead-2",
					"hidden1-0", "hidden1-1", "hidden1-2",
					"hidden2-0", "hidden2-1", "hidden2-2",
					"maintenance-0", "maintenance-1", "maintenance-2",
					"quarantined-0", "quarantined-1", "quarantined-2",
					"visible1-0", "visible1-1", "visible1-2",
					"visible2-0", "visible2-1", "visible2-2",
				},
			)
		})

		Convey("Empty state intersection", func() {
			checkQuery(
				&apipb.BotsRequest{
					IsBusy: apipb.NullableBool_TRUE,
					IsDead: apipb.NullableBool_TRUE,
				}, nil)
		})

		Convey("Simple dimension filter", func() {
			checkQuery(
				&apipb.BotsRequest{
					Dimensions: []*apipb.StringPair{
						{Key: "idx", Value: "1"},
					},
				},
				[]string{
					"busy-1",
					"dead-1",
					"hidden1-1",
					"hidden2-1",
					"maintenance-1",
					"quarantined-1",
					"visible1-1",
					"visible2-1",
				},
			)
		})

		Convey("Simple dimension filter + state filter", func() {
			checkQuery(
				&apipb.BotsRequest{
					Dimensions: []*apipb.StringPair{
						{Key: "idx", Value: "1"},
					},
					IsDead: apipb.NullableBool_TRUE,
				},
				[]string{
					"dead-1",
				},
			)
		})

		Convey("AND dimension filter", func() {
			checkQuery(
				&apipb.BotsRequest{
					Dimensions: []*apipb.StringPair{
						{Key: "idx", Value: "1"},
						{Key: "pool", Value: "visible-pool2"},
					},
				},
				[]string{
					"visible2-1",
				},
			)
		})

		Convey("Complex OR dimension filter", func() {
			checkQuery(
				&apipb.BotsRequest{
					Dimensions: []*apipb.StringPair{
						{Key: "idx", Value: "0|2|ignore"},
						{Key: "pool", Value: "visible-pool2|visible-pool1"},
					},
				},
				[]string{
					// Note: no hidden* bots here.
					"busy-0", "busy-2",
					"dead-0", "dead-2",
					"maintenance-0", "maintenance-2",
					"quarantined-0", "quarantined-2",
					"visible1-0", "visible1-2",
					"visible2-0", "visible2-2",
				},
			)
		})

		Convey("Complex OR dimension filter + state filter", func() {
			checkQuery(
				&apipb.BotsRequest{
					Dimensions: []*apipb.StringPair{
						{Key: "idx", Value: "0|2|ignore"},
						{Key: "pool", Value: "visible-pool2|visible-pool1"},
					},
					IsDead: apipb.NullableBool_TRUE,
				},
				[]string{
					"dead-0", "dead-2",
				},
			)
		})
	})

	Convey("DeathTimeout", t, func() {
		resp, err := callAsAdmin(&apipb.BotsRequest{})
		So(err, ShouldBeNil)
		So(resp.DeathTimeout, ShouldEqual, state.Configs.Settings.BotDeathTimeoutSecs)
	})
}
