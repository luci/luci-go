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
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCountBots(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)

	state := SetupTestBots(ctx)

	callImpl := func(ctx context.Context, req *apipb.BotsCountRequest) (*apipb.BotsCount, error) {
		return (&BotsServer{
			// memory.Use(...) datastore fake doesn't support IN queries currently.
			BotQuerySplitMode: model.SplitCompletely,
		}).CountBots(ctx, req)
	}
	call := func(req *apipb.BotsCountRequest) (*apipb.BotsCount, error) {
		return callImpl(MockRequestState(ctx, state), req)
	}
	callAsAdmin := func(req *apipb.BotsCountRequest) (*apipb.BotsCount, error) {
		return callImpl(MockRequestState(ctx, state.SetCaller(AdminFakeCaller)), req)
	}

	ftt.Run("Dimensions filter is checked", t, func(t *ftt.Test) {
		_, err := call(&apipb.BotsCountRequest{
			Dimensions: []*apipb.StringPair{
				{Key: "", Value: ""},
			},
		})
		assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.InvalidArgument))
	})

	ftt.Run("ACLs", t, func(t *ftt.Test) {
		t.Run("Listing only visible pools: OK", func(t *ftt.Test) {
			_, err := call(&apipb.BotsCountRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|visible-pool2"},
				},
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Listing visible and invisible pool: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.BotsCountRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|hidden-pool1"},
				},
			})
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
		})

		t.Run("Listing visible and invisible pool as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.BotsCountRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|hidden-pool1"},
				},
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Listing all pools as non-admin: permission denied", func(t *ftt.Test) {
			_, err := call(&apipb.BotsCountRequest{})
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
		})

		t.Run("Listing all pools as admin: OK", func(t *ftt.Test) {
			_, err := callAsAdmin(&apipb.BotsCountRequest{})
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run("Filtering", t, func(t *ftt.Test) {
		count := func(dims ...string) *apipb.BotsCount {
			req := &apipb.BotsCountRequest{}
			for _, kv := range dims {
				k, v, _ := strings.Cut(kv, ":")
				req.Dimensions = append(req.Dimensions, &apipb.StringPair{
					Key:   k,
					Value: v,
				})
			}
			resp, err := callAsAdmin(req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Now, should.NotBeNil)
			resp.Now = nil // for easier comparison
			return resp
		}

		// No filters.
		assert.Loosely(t, count(), should.Resemble(&apipb.BotsCount{
			Count:       24,
			Quarantined: 3,
			Maintenance: 3,
			Dead:        3,
			Busy:        3,
		}))

		// Simple filter.
		assert.Loosely(t, count("idx:1"), should.Resemble(&apipb.BotsCount{
			Count:       8,
			Quarantined: 1,
			Maintenance: 1,
			Dead:        1,
			Busy:        1,
		}))

		// AND filter.
		assert.Loosely(t, count("idx:1", "pool:visible-pool2"), should.Resemble(&apipb.BotsCount{
			Count: 1,
		}))

		// OR filter.
		assert.Loosely(t, count("idx:0|1"), should.Resemble(&apipb.BotsCount{
			Count:       16,
			Quarantined: 2,
			Maintenance: 2,
			Dead:        2,
			Busy:        2,
		}))

		// OR filter with intersecting results. This covers all bots, twice.
		assert.Loosely(t, count("idx:0|1|2", "dup:0|1|2"), should.Resemble(&apipb.BotsCount{
			Count:       24,
			Quarantined: 3,
			Maintenance: 3,
			Dead:        3,
			Busy:        3,
		}))

		// OR filter with no results.
		assert.Loosely(t, count("idx:4|5|6", "pool:visible-pool1"), should.Resemble(&apipb.BotsCount{
			Count:       0,
			Quarantined: 0,
			Maintenance: 0,
			Dead:        0,
			Busy:        0,
		}))
	})
}
