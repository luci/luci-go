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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/secrets"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

func TestListBots(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	state := SetupTestBots(ctx)

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

	ftt.Run("Limit is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotsRequest{
			Limit: -10,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		_, err = call(&apipb.BotsRequest{
			Limit: 1001,
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Cursor is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotsRequest{
			Cursor: "!!!!",
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Dimensions filter is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotsRequest{
			Dimensions: []*apipb.StringPair{
				{Key: "", Value: ""},
			},
		})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("ACLs", t, func(t *ftt.Test) {
		t.Run("Listing only visible pools: OK", func(t *ftt.Test) {
			_, err := call(&apipb.BotsRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|visible-pool2"},
				},
			})
			assert.NoErr(t, err)
		})

		t.Run("Listing visible and invisible pool: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.BotsRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|hidden-pool1"},
				},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("Listing visible and invisible pool as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.BotsRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|hidden-pool1"},
				},
			})
			assert.NoErr(t, err)
		})

		t.Run("Listing all pools as non-admin: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.BotsRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("Listing all pools as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.BotsRequest{})
			assert.NoErr(t, err)
		})
	})

	ftt.Run("Filtering and cursors", t, func(t *ftt.Test) {
		checkQuery := func(req *apipb.BotsRequest, expected []string) {
			// An unlimited query first.
			resp, err := callAsAdmin(req)
			assert.NoErr(t, err)
			assert.Loosely(t, botIDs(resp.Items), should.Match(expected))

			// A paginated one should return the same results.
			var out []*apipb.BotInfo
			var cursor string
			for {
				req.Cursor = cursor
				req.Limit = 2
				resp, err := callAsAdmin(req)
				assert.NoErr(t, err)
				assert.That(t, len(resp.Items), should.BeLessThanOrEqual(int(req.Limit)))
				out = append(out, resp.Items...)
				cursor = resp.Cursor
				if cursor == "" {
					break
				}
			}
			assert.Loosely(t, botIDs(out), should.Match(expected))
		}

		t.Run("No filters", func(t *ftt.Test) {
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

		t.Run("Busy only", func(t *ftt.Test) {
			checkQuery(
				&apipb.BotsRequest{
					IsBusy: apipb.NullableBool_TRUE,
				},
				[]string{
					"busy-0", "busy-1", "busy-2",
				},
			)
		})

		t.Run("Dead only", func(t *ftt.Test) {
			checkQuery(
				&apipb.BotsRequest{
					IsDead: apipb.NullableBool_TRUE,
				},
				[]string{
					"dead-0", "dead-1", "dead-2",
				},
			)
		})

		t.Run("Maintenance only", func(t *ftt.Test) {
			checkQuery(
				&apipb.BotsRequest{
					InMaintenance: apipb.NullableBool_TRUE,
				},
				[]string{
					"maintenance-0", "maintenance-1", "maintenance-2",
				},
			)
		})

		t.Run("Quarantined only", func(t *ftt.Test) {
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
		t.Run("Non-busy only", func(t *ftt.Test) {
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

		t.Run("Empty state intersection", func(t *ftt.Test) {
			checkQuery(
				&apipb.BotsRequest{
					IsBusy: apipb.NullableBool_TRUE,
					IsDead: apipb.NullableBool_TRUE,
				}, nil)
		})

		t.Run("Simple dimension filter", func(t *ftt.Test) {
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

		t.Run("Simple dimension filter + state filter", func(t *ftt.Test) {
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

		t.Run("AND dimension filter", func(t *ftt.Test) {
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

		t.Run("Complex OR dimension filter", func(t *ftt.Test) {
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

		t.Run("Complex OR dimension filter + state filter", func(t *ftt.Test) {
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

	ftt.Run("DeathTimeout", t, func(t *ftt.Test) {
		resp, err := callAsAdmin(&apipb.BotsRequest{})
		assert.NoErr(t, err)
		assert.Loosely(t, resp.DeathTimeout, should.Equal(state.Configs.Settings.BotDeathTimeoutSecs))
	})
}
