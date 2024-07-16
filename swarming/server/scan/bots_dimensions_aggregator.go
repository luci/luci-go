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
	"cmp"
	"context"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/model/internalmodelpb"
)

// BotsDimensionsAggregator is BotVisitor that collects the set of all possible
// bot dimensions and stores it in the datastore.
//
// The result is used by GetBotDimensions RPCs.
type BotsDimensionsAggregator struct {
	shards  []*botDimsAggregatorShardState
	current chan entityOrErr
}

type entityOrErr struct {
	res *model.BotsDimensionsAggregation
	err error
}

var _ BotVisitor = (*BotsDimensionsAggregator)(nil)

// Prepare prepares the visitor to use `shards` parallel queries.
//
// Part of BotVisitor interface.
func (a *BotsDimensionsAggregator) Prepare(ctx context.Context, shards int) {
	a.shards = make([]*botDimsAggregatorShardState, shards)
	for i := range a.shards {
		a.shards[i] = newBotDimsAggregatorShardState()
	}

	// Start fetching the existing BotsDimensionsAggregation in parallel.
	a.current = make(chan entityOrErr, 1)
	go func() {
		outcome := entityOrErr{
			res: &model.BotsDimensionsAggregation{
				Key: model.BotsDimensionsAggregationKey(ctx),
			},
			err: errors.Reason("fetch incomplete due to a panic").Err(),
		}
		defer func() { a.current <- outcome }()
		outcome.err = datastore.Get(ctx, outcome.res)
	}()
}

// Visit is called for every bot.
//
// Part of BotVisitor interface.
func (a *BotsDimensionsAggregator) Visit(ctx context.Context, shard int, bot *model.BotInfo) {
	a.shards[shard].collect(bot.Dimensions)
}

// Finalize is called once the scan is done.
//
// Part of BotVisitor interface.
func (a *BotsDimensionsAggregator) Finalize(ctx context.Context, scanErr error) error {
	// The aggregated result is invalid if the scan didn't finish successfully.
	// Skip storing it.
	if scanErr != nil {
		<-a.current // do not leave the background goroutine running
		return nil
	}

	// Merge all shards into a single set of dimensions.
	merged := a.shards[0]
	for _, shard := range a.shards[1:] {
		merged.mergeFrom(shard)
	}

	// Convert to a proto form to be stored in the datastore.
	out := &internalmodelpb.AggregatedDimensions{}
	perPool := map[string]*internalmodelpb.AggregatedDimensions_Pool{}
	for key, vals := range merged.poolMap {
		dims := perPool[key.pool]
		if dims == nil {
			dims = &internalmodelpb.AggregatedDimensions_Pool{Pool: key.pool}
			perPool[key.pool] = dims
			out.Pools = append(out.Pools, dims)
		}
		dims.Dimensions = append(dims.Dimensions, &internalmodelpb.AggregatedDimensions_Pool_Dimension{
			Name:   key.dim,
			Values: vals.ToSortedSlice(),
		})
	}

	// Within each AggregatedDimensions_Pool, sort Dimensions by name.
	for _, dims := range perPool {
		slices.SortFunc(dims.Dimensions, func(a, b *internalmodelpb.AggregatedDimensions_Pool_Dimension) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}
	// Sort AggregatedDimensions_Pool themselves by name.
	slices.SortFunc(out.Pools, func(a, b *internalmodelpb.AggregatedDimensions_Pool) int {
		return cmp.Compare(a.Pool, b.Pool)
	})

	logging.Infof(ctx, "BotsDimensionsAggregation size is %d", proto.Size(out))

	// Update the datastore record if anything has changed. Skipping writes if
	// nothing has changed reduces cache thrashing. This matters since the
	// aggregator runs every minute and clearing the cache for this large entity
	// every minute would be bad.
	current := <-a.current
	switch {
	case current.err == nil:
		if proto.Equal(out, current.res.Dimensions) {
			logging.Infof(ctx,
				"BotsDimensionsAggregation is up-to-date (last updated %s ago)",
				clock.Since(ctx, current.res.LastUpdate),
			)
			return nil
		}
	case !errors.Is(current.err, datastore.ErrNoSuchEntity):
		return errors.Annotate(current.err, "fetching current BotsDimensionsAggregation").Err()
	}

	// Update the state stored in the datastore.
	logging.Infof(ctx, "Updating BotsDimensionsAggregation...")
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		now := clock.Now(ctx).UTC()
		return datastore.Put(ctx,
			&model.BotsDimensionsAggregation{
				Key:        model.BotsDimensionsAggregationKey(ctx),
				LastUpdate: now,
				Dimensions: out,
			},
			&model.BotsDimensionsAggregationInfo{
				Key:        model.BotsDimensionsAggregationInfoKey(ctx),
				LastUpdate: now,
			},
		)
	}, nil)
	if err != nil {
		return errors.Annotate(err, "updating BotsDimensionsAggregation entity").Err()
	}
	logging.Infof(ctx, "BotsDimensionsAggregation updated")
	return nil
}

////////////////////////////////////////////////////////////////////////////////

type botDimsAggregatorShardState struct {
	// (pool, dimension key) -> set of dimension values for that dim in the pool.
	poolMap map[poolMapKey]stringset.Set
}

type poolMapKey struct {
	pool string
	dim  string
}

func newBotDimsAggregatorShardState() *botDimsAggregatorShardState {
	return &botDimsAggregatorShardState{
		poolMap: map[poolMapKey]stringset.Set{},
	}
}

// collect takes a list of "key:value" pairs with some bot's dimensions.
func (b *botDimsAggregatorShardState) collect(dims []string) {
	// Find the set of pools the bot belongs to (usually just 1 pool).
	pools := make([]string, 0, 1)
	for _, kv := range dims {
		if strings.HasPrefix(kv, "pool:") {
			pools = append(pools, strings.TrimPrefix(kv, "pool:"))
		}
	}
	if len(pools) == 0 {
		return
	}

	// Add bot dimensions to each pool's set of dimensions seen there.
	for _, kv := range dims {
		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			continue // should not happen
		}
		if isHighCardinalityDimension(k) {
			continue // skip, do not blow up the index
		}
		for _, pool := range pools {
			b.addOne(pool, k, v)
		}
	}
}

// addOne adds an entry to poolMap.
func (b *botDimsAggregatorShardState) addOne(pool, key, val string) {
	mapKey := poolMapKey{pool, key}
	if set, ok := b.poolMap[mapKey]; ok {
		set.Add(val)
	} else {
		b.poolMap[mapKey] = stringset.NewFromSlice(val)
	}
}

// mergeFrom adds all entries from 'other' into 'b', consuming 'other' in the
// process.
func (b *botDimsAggregatorShardState) mergeFrom(other *botDimsAggregatorShardState) {
	for key, vals := range other.poolMap {
		if existing, ok := b.poolMap[key]; ok {
			for val := range vals {
				existing.Add(val)
			}
		} else {
			b.poolMap[key] = vals
		}
	}
	other.poolMap = nil // we took ownership of sets stored there
}

// isHighCardinalityDimension is true if the dimension should be excluded from
// the aggregation due to having a lot of different possible values.
func isHighCardinalityDimension(key string) bool {
	// TODO(vadimsh): Make this configurable? Can also determine cardinality
	// automatically and auto-magically exclude dimensions with lots of values.
	// This will look weird though if a dimension is close to the cutting
	// threshold: sometimes it would appear in the aggregation, sometimes not.
	return key == "id" || // the bot ID itself, each bot reports it as a dimension
		key == "dut_name" || // ChromeOS-specific
		key == "dut_id" // ChromeOS-specific
}
