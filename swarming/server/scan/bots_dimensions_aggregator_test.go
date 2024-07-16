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

package scan

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/model/internalmodelpb"
)

func TestBotsDimensionsAggregator(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		bots  [][]string
		pools []*internalmodelpb.AggregatedDimensions_Pool
	}{
		{
			name: "no bots",
		},

		{
			name: "one bot",
			bots: [][]string{
				{"pool:a", "d2:v3", "d1:v2", "d1:v1", "id:skip", "dut_name:skip", "dut_id:skip"},
			},
			pools: []*internalmodelpb.AggregatedDimensions_Pool{
				{
					Pool: "a",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1", "v2"}},
						{Name: "d2", Values: []string{"v3"}},
						{Name: "pool", Values: []string{"a"}},
					},
				},
			},
		},

		{
			name: "one bot in two pools",
			bots: [][]string{{"pool:a1", "pool:a2", "d1:v1"}},
			pools: []*internalmodelpb.AggregatedDimensions_Pool{
				{
					Pool: "a1",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1"}},
						{Name: "pool", Values: []string{"a1", "a2"}},
					},
				},
				{
					Pool: "a2",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1"}},
						{Name: "pool", Values: []string{"a1", "a2"}},
					},
				},
			},
		},

		{
			name: "many bots in one pool",
			bots: [][]string{
				{"pool:a", "d1:v6", "d2:c1"},
				{"pool:a", "d1:v6", "d2:c1"},
				{"pool:a", "d1:v5", "d2:c2"},
				{"pool:a", "d1:v5", "d2:c2"},
				{"pool:a", "d1:v4", "d2:c3"},
				{"pool:a", "d1:v4", "d2:c3"},
				{"pool:a", "d1:v3", "d2:c4"},
				{"pool:a", "d1:v3", "d2:c4"},
				{"pool:a", "d1:v2", "d2:c5"},
				{"pool:a", "d1:v2", "d2:c5"},
				{"pool:a", "d1:v1", "d2:c6"},
				{"pool:a", "d1:v1", "d2:c6"},
			},
			pools: []*internalmodelpb.AggregatedDimensions_Pool{
				{
					Pool: "a",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1", "v2", "v3", "v4", "v5", "v6"}},
						{Name: "d2", Values: []string{"c1", "c2", "c3", "c4", "c5", "c6"}},
						{Name: "pool", Values: []string{"a"}},
					},
				},
			},
		},

		{
			name: "many bots in many pools",
			bots: [][]string{
				{"pool:6", "d1:v1", "d2:c1"},
				{"pool:5", "d1:v1", "d2:c1"},
				{"pool:4", "d1:v1", "d2:c1"},
				{"pool:3", "d1:v1", "d2:c1"},
				{"pool:2", "d1:v1", "d2:c1"},
				{"pool:1", "d1:v1", "d2:c1"},
			},
			pools: []*internalmodelpb.AggregatedDimensions_Pool{
				{
					Pool: "1",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1"}},
						{Name: "d2", Values: []string{"c1"}},
						{Name: "pool", Values: []string{"1"}},
					},
				},
				{
					Pool: "2",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1"}},
						{Name: "d2", Values: []string{"c1"}},
						{Name: "pool", Values: []string{"2"}},
					},
				},
				{
					Pool: "3",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1"}},
						{Name: "d2", Values: []string{"c1"}},
						{Name: "pool", Values: []string{"3"}},
					},
				},
				{
					Pool: "4",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1"}},
						{Name: "d2", Values: []string{"c1"}},
						{Name: "pool", Values: []string{"4"}},
					},
				},
				{
					Pool: "5",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1"}},
						{Name: "d2", Values: []string{"c1"}},
						{Name: "pool", Values: []string{"5"}},
					},
				},
				{
					Pool: "6",
					Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
						{Name: "d1", Values: []string{"v1"}},
						{Name: "d2", Values: []string{"c1"}},
						{Name: "pool", Values: []string{"6"}},
					},
				},
			},
		},
	}

	for _, cs := range cases {
		cs := cs
		t.Run(cs.name, func(t *testing.T) {
			t.Parallel()
			ctx := memory.Use(context.Background())
			got := runBotsDimsAggregation(ctx, t, cs.bots)
			want := &internalmodelpb.AggregatedDimensions{Pools: cs.pools}
			if diff := cmp.Diff(want, got.Dimensions, protocmp.Transform()); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		})
	}
}

func TestBotsDimensionsAggregatorUpdateFlow(t *testing.T) {
	t.Parallel()

	testTime := testclock.TestRecentTimeUTC.Round(time.Second)

	ctx := memory.Use(context.Background())
	ctx, tc := testclock.UseTime(ctx, testTime)

	// The initial run creates an empty entity.
	agg := runBotsDimsAggregation(ctx, t, nil)
	if !agg.LastUpdate.Equal(testTime) {
		t.Fatalf("want %s, got %s", testTime, agg.LastUpdate)
	}
	if len(agg.Dimensions.Pools) != 0 {
		t.Fatalf("unexpected pools: %s", agg.Dimensions.Pools)
	}

	// Skips updating it if there are still no bots.
	tc.Add(time.Hour)
	agg = runBotsDimsAggregation(ctx, t, nil)
	if !agg.LastUpdate.Equal(testTime) {
		t.Fatalf("want %s, got %s", testTime, agg.LastUpdate)
	}

	// Updates it if bots appears.
	botsSet := [][]string{
		{"pool:a", "dim:val1"},
	}
	agg = runBotsDimsAggregation(ctx, t, botsSet)
	if want := testTime.Add(time.Hour); !agg.LastUpdate.Equal(want) {
		t.Fatalf("want %s, got %s", want, agg.LastUpdate)
	}

	// No changes if the set of dimensions hasn't changes.
	tc.Add(time.Hour)
	botsSet = append(botsSet, [][]string{
		{"pool:a", "dim:val1"}, // seen this dimension already
	}...)
	agg = runBotsDimsAggregation(ctx, t, botsSet)
	if want := testTime.Add(time.Hour); !agg.LastUpdate.Equal(want) {
		t.Fatalf("want %s, got %s", want, agg.LastUpdate)
	}

	// Updates if it the set of bots changes.
	botsSet = append(botsSet, [][]string{
		{"pool:a", "dim:val2"},
	}...)
	agg = runBotsDimsAggregation(ctx, t, botsSet)
	if want := testTime.Add(2 * time.Hour); !agg.LastUpdate.Equal(want) {
		t.Fatalf("want %s, got %s", want, agg.LastUpdate)
	}
}

func runBotsDimsAggregation(ctx context.Context, t *testing.T, bots [][]string) *model.BotsDimensionsAggregation {
	var fakeBots []FakeBot
	for idx, dims := range bots {
		fakeBots = append(fakeBots, FakeBot{
			ID:   fmt.Sprintf("bot-%d", idx),
			Dims: dims,
		})
	}
	err := RunBotVisitor(ctx, &BotsDimensionsAggregator{}, fakeBots)
	if err != nil {
		t.Fatal(err)
	}
	ent := model.BotsDimensionsAggregation{
		Key: model.BotsDimensionsAggregationKey(ctx),
	}
	info := model.BotsDimensionsAggregationInfo{
		Key: model.BotsDimensionsAggregationInfoKey(ctx),
	}
	if err = datastore.Get(ctx, &ent, &info); err != nil {
		t.Fatal(err)
	}
	if !ent.LastUpdate.Equal(info.LastUpdate) {
		t.Fatalf(
			"BotsDimensionsAggregation.LastUpdate (%s) != BotsDimensionsAggregationInfo.LastUpdate (%s)",
			ent.LastUpdate, info.LastUpdate,
		)
	}
	return &ent
}
