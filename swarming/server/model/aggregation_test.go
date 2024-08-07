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

package model

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model/internalmodelpb"
)

func TestBotsDimensionsSets(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	testTimeTS := timestamppb.New(testTime)

	ftt.Run("Works", t, func(t *ftt.Test) {
		bs := NewBotsDimensionsSets([]*internalmodelpb.AggregatedDimensions_Pool{
			{
				Pool: "p1",
				Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
					{Name: "d1", Values: []string{"v1", "v2"}},
					{Name: "d2", Values: []string{"c1", "c2"}},
				},
			},
			{
				Pool: "p2",
				Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
					{Name: "d1", Values: []string{"v2", "v3"}},
					{Name: "d2", Values: []string{"c2", "c3"}},
					{Name: "d3", Values: []string{"a1", "a2"}},
				},
			},
			{
				Pool: "p3",
				Dimensions: []*internalmodelpb.AggregatedDimensions_Pool_Dimension{
					{Name: "d1", Values: []string{"v3", "v4"}},
					{Name: "d2", Values: []string{"c3", "c4"}},
					{Name: "d3", Values: []string{"a2", "a3"}},
					{Name: "d4", Values: []string{"b1", "b2"}},
				},
			},
		}, testTime)

		assert.Loosely(t, bs.DimensionsGlobally(), should.Resemble(&apipb.BotsDimensions{
			BotsDimensions: []*apipb.StringListPair{
				{Key: "d1", Value: []string{"v1", "v2", "v3", "v4"}},
				{Key: "d2", Value: []string{"c1", "c2", "c3", "c4"}},
				{Key: "d3", Value: []string{"a1", "a2", "a3"}},
				{Key: "d4", Value: []string{"b1", "b2"}},
			},
			Ts: testTimeTS,
		}))

		assert.Loosely(t, bs.DimensionsInPools(nil), should.Resemble(&apipb.BotsDimensions{
			Ts: testTimeTS,
		}))
		assert.Loosely(t, bs.DimensionsInPools([]string{"unknown"}), should.Resemble(&apipb.BotsDimensions{
			Ts: testTimeTS,
		}))
		assert.Loosely(t, bs.DimensionsInPools([]string{"unknown1", "unknown2"}), should.Resemble(&apipb.BotsDimensions{
			Ts: testTimeTS,
		}))

		assert.Loosely(t, bs.DimensionsInPools([]string{"p1"}), should.Resemble(&apipb.BotsDimensions{
			BotsDimensions: []*apipb.StringListPair{
				{Key: "d1", Value: []string{"v1", "v2"}},
				{Key: "d2", Value: []string{"c1", "c2"}},
			},
			Ts: testTimeTS,
		}))

		assert.Loosely(t, bs.DimensionsInPools([]string{"p1", "p2", "unknown"}), should.Resemble(&apipb.BotsDimensions{
			BotsDimensions: []*apipb.StringListPair{
				{Key: "d1", Value: []string{"v1", "v2", "v3"}},
				{Key: "d2", Value: []string{"c1", "c2", "c3"}},
				{Key: "d3", Value: []string{"a1", "a2"}},
			},
			Ts: testTimeTS,
		}))
	})
}

func TestBotsDimensionsCache(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, tc := testclock.UseTime(ctx, testTime)

		updateDS := func(lastUpdate time.Time) {
			err := datastore.Put(ctx,
				&BotsDimensionsAggregation{
					Key:        BotsDimensionsAggregationKey(ctx),
					LastUpdate: lastUpdate,
					Dimensions: &internalmodelpb.AggregatedDimensions{}, // not used in this test
				},
				&BotsDimensionsAggregationInfo{
					Key:        BotsDimensionsAggregationInfoKey(ctx),
					LastUpdate: lastUpdate,
				},
			)
			assert.Loosely(t, err, should.BeNil)
		}

		var cache BotsDimensionsCache

		// Getting the initial copy fails, since there's nothing in datastore yet.
		_, err := cache.Get(ctx)
		assert.Loosely(t, errors.Is(err, datastore.ErrNoSuchEntity), should.BeTrue)

		// Put something and try again. It should work now.
		expectedUpdate1 := testTime.Add(-5 * time.Hour)
		updateDS(expectedUpdate1)
		set1, err := cache.Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, set1.lastUpdate.Equal(expectedUpdate1), should.BeTrue)

		// At a later time returns the exact same object since nothing has changed.
		tc.Add(time.Hour)
		set2, err := cache.Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, set2 == set1, should.BeTrue) // equal pointers

		// Datastore is updated, but we are still using a cached copy since it
		// hasn't expired yet.
		tc.Add(50 * time.Second)
		expectedUpdate2 := clock.Now(ctx).UTC()
		updateDS(expectedUpdate2)
		set3, err := cache.Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, set3 == set1, should.BeTrue) // equal pointers

		// Few seconds later the cached copy has expired and we got a new one.
		tc.Add(11 * time.Second)
		set4, err := cache.Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, set4 != set1, should.BeTrue)
		assert.Loosely(t, set4.lastUpdate.Equal(expectedUpdate2), should.BeTrue)
	})
}
