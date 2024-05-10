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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("Dimensions filter is checked", t, func() {
		_, err := call(&apipb.BotsCountRequest{
			Dimensions: []*apipb.StringPair{
				{Key: "", Value: ""},
			},
		})
		So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
	})

	Convey("ACLs", t, func() {
		Convey("Listing only visible pools: OK", func() {
			_, err := call(&apipb.BotsCountRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|visible-pool2"},
				},
			})
			So(err, ShouldBeNil)
		})

		Convey("Listing visible and invisible pool: permission denied", func() {
			_, err := call(&apipb.BotsCountRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|hidden-pool1"},
				},
			})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		})

		Convey("Listing visible and invisible pool as admin: OK", func() {
			_, err := callAsAdmin(&apipb.BotsCountRequest{
				Dimensions: []*apipb.StringPair{
					{Key: "pool", Value: "visible-pool1|hidden-pool1"},
				},
			})
			So(err, ShouldBeNil)
		})

		Convey("Listing all pools as non-admin: permission denied", func() {
			_, err := call(&apipb.BotsCountRequest{})
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		})

		Convey("Listing all pools as admin: OK", func() {
			_, err := callAsAdmin(&apipb.BotsCountRequest{})
			So(err, ShouldBeNil)
		})
	})

	Convey("Filtering", t, func() {
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
			So(err, ShouldBeNil)
			So(resp.Now, ShouldNotBeNil)
			resp.Now = nil // for easier comparison
			return resp
		}

		// No filters.
		So(count(), ShouldResembleProto, &apipb.BotsCount{
			Count:       24,
			Quarantined: 3,
			Maintenance: 3,
			Dead:        3,
			Busy:        3,
		})

		// Simple filter.
		So(count("idx:1"), ShouldResembleProto, &apipb.BotsCount{
			Count:       8,
			Quarantined: 1,
			Maintenance: 1,
			Dead:        1,
			Busy:        1,
		})

		// AND filter.
		So(count("idx:1", "pool:visible-pool2"), ShouldResembleProto, &apipb.BotsCount{
			Count: 1,
		})

		// OR filter.
		So(count("idx:0|1"), ShouldResembleProto, &apipb.BotsCount{
			Count:       16,
			Quarantined: 2,
			Maintenance: 2,
			Dead:        2,
			Busy:        2,
		})

		// OR filter with intersecting results. This covers all bots, twice.
		So(count("idx:0|1|2", "dup:0|1|2"), ShouldResembleProto, &apipb.BotsCount{
			Count:       24,
			Quarantined: 3,
			Maintenance: 3,
			Dead:        3,
			Busy:        3,
		})

		// OR filter with no results.
		So(count("idx:4|5|6", "pool:visible-pool1"), ShouldResembleProto, &apipb.BotsCount{
			Count:       0,
			Quarantined: 0,
			Maintenance: 0,
			Dead:        0,
			Busy:        0,
		})
	})
}
