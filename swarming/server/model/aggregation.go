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
	"cmp"
	"context"
	"slices"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/caching/lazyslot"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model/internalmodelpb"
)

// BotsDimensionsAggregation stores all observed bot dimensions per pool.
//
// It is updated by scan.BotsDimensionsAggregator.
type BotsDimensionsAggregation struct {
	_ datastore.PropertyMap `gae:"-,extra"`

	// Key is BotsDimensionsAggregationKey.
	Key *datastore.Key `gae:"$key"`
	// LastUpdate is when this entity changed the last time.
	LastUpdate time.Time `gae:",noindex"`

	// Dimensions is actual aggregated dimensions map.
	//
	// Stored internally as zstd-compressed proto (since the datastore library
	// compressed protos by default if they are larger than some threshold).
	Dimensions *internalmodelpb.AggregatedDimensions
}

// BotsDimensionsAggregationInfo contains info about BotsDimensionsAggregation.
//
// It is tiny in comparison. Its LastUpdate field can be used to quickly check
// if the aggregated dimensions set has changed and needs to be reloaded. Both
// entities are always updated in the same transaction.
//
// It is updated by scan.BotsDimensionsAggregator.
type BotsDimensionsAggregationInfo struct {
	_ datastore.PropertyMap `gae:"-,extra"`

	// Key is BotsDimensionsAggregationInfoKey.
	Key *datastore.Key `gae:"$key"`
	// LastUpdate is when BotsDimensionsAggregation changed the last time.
	LastUpdate time.Time `gae:",noindex"`
}

// BotsDimensionsAggregationKey is BotsDimensionsAggregation entity key.
func BotsDimensionsAggregationKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "BotsDimensionsAggregation", "", 1, nil)
}

// BotsDimensionsAggregationInfoKey is BotsDimensionsAggregationInfo entity key.
func BotsDimensionsAggregationInfoKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "BotsDimensionsAggregationInfo", "", 1, BotsDimensionsAggregationKey(ctx))
}

// BotsDimensionsSets is a parsed representation of BotsDimensionsAggregation.
//
// Given a list of pools it can return a set of all observed bot dimensions in
// these pools.
//
// Immutable.
type BotsDimensionsSets struct {
	// lastUpdate matches BotsDimensionsAggregation.LastUpdate.
	lastUpdate time.Time
	// Pool name => [(dimension key, [val1, val2, ...]), ...].
	perPool map[string][]*apipb.StringListPair
	// Precalculated union of all pools (it is fetched relatively often).
	global []*apipb.StringListPair
}

// NewBotsDimensionsSets constructs BotsDimensionsSets based on aggregated data.
//
// Assumes dimensions values are sorted lists already, as produced by
// scan.BotsDimensionsAggregator.
func NewBotsDimensionsSets(perPool []*internalmodelpb.AggregatedDimensions_Pool, lastUpdate time.Time) *BotsDimensionsSets {
	// A list of all known pools to be able to reuse unionOfPerPoolDims to get
	// the set of **all** known dimensions.
	allPools := make([]string, 0, len(perPool))
	// Pool name => dimensions there in a form closer to the output API.
	perPoolLists := make(map[string][]*apipb.StringListPair, len(perPool))
	for _, pool := range perPool {
		dims := make([]*apipb.StringListPair, len(pool.Dimensions))
		for i, dim := range pool.Dimensions {
			dims[i] = &apipb.StringListPair{
				Key:   dim.Name,
				Value: dim.Values,
			}
		}
		perPoolLists[pool.Pool] = dims
		allPools = append(allPools, pool.Pool)
	}
	return &BotsDimensionsSets{
		lastUpdate: lastUpdate,
		perPool:    perPoolLists,
		global:     unionOfPerPoolDims(perPoolLists, allPools),
	}
}

// DimensionsInPools returns a set of dimensions of bots in all given pools.
//
// Unknown pools are considered empty.
func (s *BotsDimensionsSets) DimensionsInPools(pools []string) *apipb.BotsDimensions {
	return &apipb.BotsDimensions{
		BotsDimensions: unionOfPerPoolDims(s.perPool, pools),
		Ts:             timestamppb.New(s.lastUpdate),
	}
}

// DimensionsGlobally returns a set of dimensions across all known pools.
func (s *BotsDimensionsSets) DimensionsGlobally() *apipb.BotsDimensions {
	return &apipb.BotsDimensions{
		BotsDimensions: s.global,
		Ts:             timestamppb.New(s.lastUpdate),
	}
}

// unionOfPerPoolDims returns a `union(perPool[p] for p in pools)`.
func unionOfPerPoolDims(perPool map[string][]*apipb.StringListPair, pools []string) []*apipb.StringListPair {
	// This can happen if a user can't see any pools at all.
	if len(pools) == 0 {
		return nil
	}

	// Skip doing a union if asked for only one pool.
	if len(pools) == 1 {
		return perPool[pools[0]] // will be empty list if the pool is unknown
	}

	// Dimension key => set of dimension values.
	perDim := map[string]stringset.Set{}

	// Add all per pool sets to the final resulting set.
	for _, pool := range pools {
		for _, dim := range perPool[pool] {
			if vals := perDim[dim.Key]; vals != nil {
				vals.AddAll(dim.Value)
			} else {
				perDim[dim.Key] = stringset.NewFromSlice(dim.Value...)
			}
		}
	}

	// Convert to a sorted proto form.
	dims := make([]*apipb.StringListPair, 0, len(perDim))
	for key, vals := range perDim {
		dims = append(dims, &apipb.StringListPair{
			Key:   key,
			Value: vals.ToSortedSlice(),
		})
	}
	slices.SortFunc(dims, func(a, b *apipb.StringListPair) int {
		return cmp.Compare(a.Key, b.Key)
	})

	return dims
}

// BotsDimensionsCache maintain an in-process cache of aggregated bot dimensions
// sets.
//
// Since aggregated bot dimensions are used relatively infrequently, the cache
// is initialized and refreshed lazily on use (instead of e.g. periodically in
// background).
type BotsDimensionsCache struct {
	cached lazyslot.Slot
}

// Get returns the aggregated bot dimensions, fetching or refreshing it if
// necessary.
//
// All returned errors should be treated as transient datastore errors.
func (b *BotsDimensionsCache) Get(ctx context.Context) (*BotsDimensionsSets, error) {
	val, err := b.cached.Get(ctx, func(prev any) (updated any, exp time.Duration, err error) {
		var fresh *BotsDimensionsSets
		if prev == nil {
			fresh, err = b.fetch(ctx)
		} else {
			fresh, err = b.refresh(ctx, prev.(*BotsDimensionsSets))
		}
		if err != nil {
			return nil, 0, err
		}
		return fresh, time.Minute, nil // cache in memory for 1 min
	})
	if err != nil {
		return nil, err
	}
	return val.(*BotsDimensionsSets), nil
}

// fetch fetches the full copy of BotsDimensionsSets.
func (b *BotsDimensionsCache) fetch(ctx context.Context) (*BotsDimensionsSets, error) {
	// Fetch the entity. It must be present (it is created continuously by a
	// cron job). Missing entity is a transient error.
	logging.Infof(ctx, "Fetching BotsDimensionsAggregation...")
	ent := &BotsDimensionsAggregation{Key: BotsDimensionsAggregationKey(ctx)}
	if err := datastore.Get(ctx, ent); err != nil {
		return nil, errors.Annotate(err, "fetching BotsDimensionsAggregation").Err()
	}
	return NewBotsDimensionsSets(ent.Dimensions.GetPools(), ent.LastUpdate), nil
}

// refresh fetches a new version of BotsDimensionsSets if necessary.
//
// Returns `prev` itself if it is already up-to-date.
func (b *BotsDimensionsCache) refresh(ctx context.Context, prev *BotsDimensionsSets) (*BotsDimensionsSets, error) {
	// Check the timestamp of the latest entity in the datastore. This fetches
	// tiny BotsDimensionsAggregationInfo entity.
	logging.Infof(ctx, "Checking if there's newer BotsDimensionsAggregation...")
	info := &BotsDimensionsAggregationInfo{Key: BotsDimensionsAggregationInfoKey(ctx)}
	switch err := datastore.Get(ctx, info); {
	case err != nil:
		return nil, errors.Annotate(err, "fetching BotsDimensionsAggregationInfo").Err()
	case info.LastUpdate.Equal(prev.lastUpdate):
		return prev, nil // can reuse the existing copy, it is still fresh
	default:
		return b.fetch(ctx) // fetch a new fresher copy
	}
}
