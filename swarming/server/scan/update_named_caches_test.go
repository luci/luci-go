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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"
)

func TestNamedCachesAggregator(t *testing.T) {
	t.Parallel()
	testTime := testclock.TestRecentTimeUTC.Round(time.Second)

	ctx := memory.Use(context.Background())
	ctx, tc := testclock.UseTime(ctx, testTime)
	datastore.GetTestable(ctx).Consistent(true)
	datastore.GetTestable(ctx).AutoIndex(true)

	cases := []struct {
		name string
		bots []FakeBot
		// Initial Datastore state before operation.
		dsInitState []*model.NamedCacheStats
		// Wanted Datastore state after operation.
		dsWantedState []*model.NamedCacheStats
	}{
		{
			name: "no bots",
		},
		{
			name: "one bot with new entry",
			bots: []FakeBot{
				{
					ID: "bot-1",
					Dims: []string{
						"pool:pool-1",
						"os:os-1",
					},
					// cache-2 is a new entry and should be reflected in DS.
					State: []byte(`{
						"named_caches": {
							"cache-1": [
								[
									"Zs",
									26848764602
								],
								1708725132
							],
							"cache-2": [
								[
									"Zs",
									84930485302
								],
								1708725132
							]
						}
					}`),
				},
			},
			dsInitState: []*model.NamedCacheStats{
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-1", "cache-1"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-1",
							Size:       40182097935,
							LastUpdate: testTime,
							ExpireAt:   testTime.Add(2 * time.Hour * 24),
						},
					},
					ExpireAt: testTime.Add(2 * time.Hour * 24),
				},
			},
			dsWantedState: []*model.NamedCacheStats{
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-1", "cache-1"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-1",
							Size:       38128764601,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
					},
					ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
					LastUpdate: testTime.Add(time.Hour),
				},
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-1", "cache-2"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-1",
							Size:       84930485302,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
					},
					ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
					LastUpdate: testTime.Add(time.Hour),
				},
			},
		},
		{
			name: "many bots with many pools",
			bots: []FakeBot{
				{
					ID: "bot-1",
					Dims: []string{
						"pool:pool-10",
						"pool:pool-20",
						"os:os-1",
					},
					State: []byte(`{
						"named_caches": {
							"cache-10": [
								[
									"Zs",
									26848764602
								],
								1708725132
							],
							"cache-20": [
								[
									"Zs",
									84930485302
								],
								1708725132
							]
						}
					}`),
				},
				{
					ID: "bot-2",
					Dims: []string{
						"pool:pool-10",
						"pool:pool-20",
						"os:os-2",
					},
					State: []byte(`{
						"named_caches": {
							"cache-10": [
								[
									"Zs",
									26848764602
								],
								1708725132
							],
							"cache-20": [
								[
									"Zs",
									84930485302
								],
								1708725132
							],
							"cache-30": [
								[
									"Zs",
									50930485302
								],
								1708725132
							]
						}
					}`),
				},
				{
					ID: "bot-3",
					Dims: []string{
						"pool:pool-10",
						"os:os-1",
					},
					State: []byte(`{
						"named_caches": {
							"cache-10": [
								[
									"Zs",
									88848764602
								],
								1708725132
							],
							"cache-20": [
								[
									"Zs",
									64930485302
								],
								1708725132
							]
						}
					}`),
				},
			},
			dsWantedState: []*model.NamedCacheStats{
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-10", "cache-10"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-1",
							Size:       88848764602,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
						{
							Name:       "os-2",
							Size:       26848764602,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
					},
					ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
					LastUpdate: testTime.Add(time.Hour),
				},
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-10", "cache-20"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-1",
							Size:       84930485302,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
						{
							Name:       "os-2",
							Size:       84930485302,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
					},
					ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
					LastUpdate: testTime.Add(time.Hour),
				},
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-20", "cache-10"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-1",
							Size:       26848764602,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
						{
							Name:       "os-2",
							Size:       26848764602,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
					},
					ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
					LastUpdate: testTime.Add(time.Hour),
				},
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-20", "cache-20"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-1",
							Size:       84930485302,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
						{
							Name:       "os-2",
							Size:       84930485302,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
					},
					ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
					LastUpdate: testTime.Add(time.Hour),
				},
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-10", "cache-30"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-2",
							Size:       50930485302,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
					},
					ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
					LastUpdate: testTime.Add(time.Hour),
				},
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-20", "cache-30"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-2",
							Size:       50930485302,
							LastUpdate: testTime.Add(time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
						},
					},
					ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
					LastUpdate: testTime.Add(time.Hour),
				},
			},
		},
		{
			name: "expired PerOSEntries should be removed",
			bots: []FakeBot{
				{
					ID: "bot-1",
					Dims: []string{
						"pool:pool-100",
						"os:os-1",
					},
					State: []byte(`{
							"named_caches": {
								"cache-200": [
									[
										"Zs",
										26848764602
									],
									1708725132
								]
							}
						}`),
				},
			},
			dsInitState: []*model.NamedCacheStats{
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-100", "cache-200"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-1",
							Size:       25848764602,
							LastUpdate: testTime,
							ExpireAt:   testTime.Add(-2 * time.Hour * 24),
						},
						{
							Name:       "os-2",
							Size:       26848764602,
							LastUpdate: testTime,
							ExpireAt:   testTime.Add(time.Hour * 24),
						},
						// Expired and non-updated entry should be removed.
						{
							Name:       "os-3",
							Size:       26848764602,
							LastUpdate: testTime,
							ExpireAt:   testTime.Add(-2 * time.Hour * 24),
						},
					},
					ExpireAt: testTime.Add(-2 * time.Hour * 24),
				},
			},
			dsWantedState: []*model.NamedCacheStats{
				{
					Key: model.NamedCacheStatsKey(ctx, "pool-100", "cache-200"),
					OS: []model.PerOSEntry{
						{
							Name:       "os-1",
							Size:       26848764602,
							LastUpdate: testTime.Add(1 * time.Hour),
							ExpireAt:   testTime.Add(8*time.Hour*24 + 1*time.Hour),
						},
						{
							Name:       "os-2",
							Size:       26848764602,
							LastUpdate: testTime,
							ExpireAt:   testTime.Add(time.Hour * 24),
						},
					},
					ExpireAt:   testTime.Add(8*time.Hour*24 + time.Hour),
					LastUpdate: testTime.Add(1 * time.Hour),
				},
			},
		},
	}
	// Ensure update times are in the past.
	tc.Add(time.Hour)
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			t.Parallel()

			// Set Datastore state.
			if err := datastore.Put(ctx, cs.dsInitState); err != nil {
				t.Fatal(err)
			}
			// Run operation.
			if err := RunBotVisitor(ctx, &NamedCachesAggregator{}, cs.bots); err != nil {
				t.Fatal(err)
			}
			// Retrieve new state.
			toGet := copyKeys(cs.dsWantedState)
			if err := datastore.Get(ctx, toGet); err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(cs.dsWantedState, toGet, cmpopts.EquateEmpty(), cmpopts.SortSlices(func(a, b model.PerOSEntry) bool {
				return a.Name < b.Name
			})); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}

		})
	}
}

func TestEMACompute(t *testing.T) {
	t.Parallel()

	// See https://en.wikipedia.org/wiki/Exponential_smoothing#Time_constant for
	// where 0.632 comes from.
	const impulse = 100_000_000
	const threshold = int64(float64(impulse * 0.632))

	ema := int64(1)
	time := 0

	for {
		time++
		ema = computeEMA(impulse, ema)
		if ema > threshold {
			break
		}
	}

	if expect := 6; time != expect {
		t.Fatalf("expect time %d, got %d", expect, time)
	}
}

// copyKeys copies stats' keys into a new NamedCacheStats slice.
func copyKeys(stats []*model.NamedCacheStats) []*model.NamedCacheStats {
	keys := make([]*model.NamedCacheStats, len(stats))
	for i, stat := range stats {
		keys[i] = &model.NamedCacheStats{
			Key: stat.Key,
		}
	}
	return keys
}
