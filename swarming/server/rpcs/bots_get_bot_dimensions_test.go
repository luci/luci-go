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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/model/internalmodelpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetBotDimensions(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	state := NewMockedRequestState()
	state.MockPool("visible-pool1", "project:visible-realm")
	state.MockPool("visible-pool2", "project:visible-realm")
	state.MockPool("hidden-pool", "project:hidden-realm")
	state.MockPerm("project:visible-realm", acls.PermPoolsListBots)

	err := datastore.Put(ctx,
		&model.BotsDimensionsAggregation{
			Key:        model.BotsDimensionsAggregationKey(ctx),
			LastUpdate: TestTime,
			Dimensions: &internalmodelpb.AggregatedDimensions{
				Pools: []*internalmodelpb.AggregatedDimensions_Pool{
					{
						Pool: "visible-pool1",
						Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
							{Name: "d1", Values: []string{"v"}},
						},
					},
					{
						Pool: "visible-pool2",
						Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
							{Name: "d2", Values: []string{"v"}},
						},
					},
					{
						Pool: "hidden-pool",
						Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
							{Name: "d3", Values: []string{"v"}},
						},
					},
					{
						Pool: "deleted-pool", // not in the pools.cfg
						Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
							{Name: "d4", Values: []string{"v"}},
						},
					},
				},
			},
		},
		&model.BotsDimensionsAggregationInfo{
			Key:        model.BotsDimensionsAggregationInfoKey(ctx),
			LastUpdate: TestTime,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	call := func(pool string) (*apipb.BotsDimensions, error) {
		ctx := MockRequestState(ctx, state)
		return (&BotsServer{}).GetBotDimensions(ctx, &apipb.BotsDimensionsRequest{Pool: pool})
	}
	callAsAdmin := func(pool string) (*apipb.BotsDimensions, error) {
		ctx := MockRequestState(ctx, state.SetCaller(AdminFakeCaller))
		return (&BotsServer{}).GetBotDimensions(ctx, &apipb.BotsDimensionsRequest{Pool: pool})
	}

	Convey("Concrete pool: visible pool", t, func() {
		out, err := call("visible-pool1")
		So(err, ShouldBeNil)
		So(out, ShouldResembleProto, &apipb.BotsDimensions{
			BotsDimensions: []*apipb.StringListPair{
				{Key: "d1", Value: []string{"v"}},
			},
			Ts: timestamppb.New(TestTime),
		})
	})

	Convey("Concrete pool: no permission", t, func() {
		_, err := call("hidden-pool")
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
	})

	Convey("Concrete pool: unknown pool", t, func() {
		_, err := call("unknown-pool")
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
	})

	Convey("All pools: admin", t, func() {
		out, err := callAsAdmin("")
		So(err, ShouldBeNil)
		So(out, ShouldResembleProto, &apipb.BotsDimensions{
			BotsDimensions: []*apipb.StringListPair{
				{Key: "d1", Value: []string{"v"}},
				{Key: "d2", Value: []string{"v"}},
				{Key: "d3", Value: []string{"v"}},
				{Key: "d4", Value: []string{"v"}},
			},
			Ts: timestamppb.New(TestTime),
		})
	})

	Convey("All pools: non-admin", t, func() {
		out, err := call("")
		So(err, ShouldBeNil)
		So(out, ShouldResembleProto, &apipb.BotsDimensions{
			BotsDimensions: []*apipb.StringListPair{
				{Key: "d1", Value: []string{"v"}},
				{Key: "d2", Value: []string{"v"}},
			},
			Ts: timestamppb.New(TestTime),
		})
	})
}
