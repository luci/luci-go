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
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"
)

const (
	// namedCachesExpiryTime is how long NamedCacheStats are kept in datastore.
	namedCachesExpiryTime = 8 * 24 * time.Hour
	// namedCachesUpdateFrequency is how often NamedCachesAggregator runs.
	namedCachesUpdateFrequency = 10 * time.Minute
	// percentileThreshold is the percentile of the cache sizes to use as a hint.
	percentileThreshold = 0.95
	// batchSize is the number of entities to process in a single batch.
	batchSize = 500
)

// NamedCachesAggregator is a BotVisitor that collects
// all named_caches present on the bot and their respective size.
type NamedCachesAggregator struct {
	shards []*namedCachesAggregatorShardState
}

var _ BotVisitor = (*NamedCachesAggregator)(nil)

// ID returns an unique identifier of this visitor used for storing its state.
//
// Part of BotVisitor interface.
func (*NamedCachesAggregator) ID() string {
	return "NamedCachesAggregator"
}

// Frequency returns how frequently this visitor should run.
//
// Part of BotVisitor interface.
func (*NamedCachesAggregator) Frequency() time.Duration {
	return namedCachesUpdateFrequency
}

// Prepare prepares the visitor to use `shards` parallel queries.
//
// Part of BotVisitor interface.
func (a *NamedCachesAggregator) Prepare(ctx context.Context, shards int, lastRun time.Time) {
	a.shards = make([]*namedCachesAggregatorShardState, shards)
	for i := range a.shards {
		a.shards[i] = newNamedCachesAggregatorShardState()
	}
}

// Visit is called for every bot.
//
// Part of BotVisitor interface.
func (a *NamedCachesAggregator) Visit(ctx context.Context, shard int, bot *model.BotInfo) {
	a.shards[shard].collect(ctx, bot)
}

// Finalize is called once the scan is done.
//
// Part of BotVisitor interface.
func (a *NamedCachesAggregator) Finalize(ctx context.Context, scanErr error) error {
	// Skip updating NamedCacheStats if the scan did not finish properly. The
	// error was already reported, so just skip the update silently.
	if scanErr != nil {
		return nil
	}

	// Merge all shards together.
	merged := a.shards[0]
	for _, shard := range a.shards[1:] {
		merged.mergeFrom(shard)
	}

	now := clock.Now(ctx).UTC()
	expireAt := now.Add(namedCachesExpiryTime)

	// Calculate stats that would need to be stored, as (pool, cache) => entity.
	stats := map[namedCacheKey]*model.NamedCacheStats{}
	for key, sizes := range merged.namedCacheMap {
		ent := stats[key.withoutOS()]
		if ent == nil {
			ent = &model.NamedCacheStats{
				Key:        model.NamedCacheStatsKey(ctx, key.pool, key.cache),
				LastUpdate: now,
				ExpireAt:   expireAt,
			}
			stats[key.withoutOS()] = ent
		}

		// Calculate the up-to-date cache sizes hint as a percentile.
		slices.Sort(sizes)
		sizeHint := sizes[int(float32(len(sizes))*percentileThreshold)]

		// Note: these entries will be sorted when storing the entity.
		ent.OS = append(ent.OS, model.PerOSEntry{
			Name:       key.os,
			Size:       sizeHint,
			LastUpdate: now,
			ExpireAt:   expireAt,
		})
	}

	logging.Infof(ctx, "Calculating named cache size percentiles took %s", clock.Since(ctx, now))

	// Apply new values to the existing datastore state in batches to avoid
	// exceeding datastore limits or out-of-memory errors. All updates are
	// independent, we can apply as much of them as possible, collecting all
	// errors and reporting them in the end.
	var gmerr errors.MultiError
	for batch := range slices.Chunk(slices.Collect(maps.Values(stats)), batchSize) {
		// Fetch corresponding existing entities. Some may be missing.
		existing := make([]*model.NamedCacheStats, len(batch))
		for i, ent := range batch {
			existing[i] = &model.NamedCacheStats{Key: ent.Key}
		}
		var merr errors.MultiError
		if err := datastore.Get(ctx, existing); err != nil && !errors.As(err, &merr) {
			// `err` is not a MultiError in the case of an overall datastore failure
			// (e.g. a time out). Skip the problematic batch and store the error in
			// the global MultiError.
			gmerr = append(gmerr, errors.Annotate(err, "fetching %d NamedCacheStats entities", len(batch)).Err())
			continue
		}

		// "Merge" values of existing entities and entities we want to store.
		var updated []*model.NamedCacheStats
		for i, existingEnt := range existing {
			if len(merr) != 0 {
				if err := merr[i]; err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
					gmerr = append(gmerr, errors.Annotate(err, "fetching NamedCacheStats entity: %s", existingEnt.Key.StringID()).Err())
					continue
				}
			}
			// Note that existingEnt.OS is empty for missing entities.
			if len(existingEnt.OS) != 0 {
				batch[i].OS = updatePerOSEntries(now, batch[i].OS, existingEnt.OS)
			}
			// Sort by OS name to make sure datastore entities all look neat.
			slices.SortFunc(batch[i].OS, func(a, b model.PerOSEntry) int {
				return strings.Compare(a.Name, b.Name)
			})
			updated = append(updated, batch[i])
		}

		// Store updated values.
		if len(updated) > 0 {
			logging.Infof(ctx, "Storing a batch with %d NamedCacheStats entities", len(updated))
			if err := datastore.Put(ctx, updated); err != nil {
				gmerr = append(gmerr, errors.Annotate(err, "updating NamedCacheStats entities").Err())
			}
		}
	}

	return gmerr.AsError()
}

////////////////////////////////////////////////////////////////////////////////////////////

type namedCachesAggregatorShardState struct {
	// The list of all observed cache sizes grouped by (pool, cache, os).
	namedCacheMap map[namedCacheKey][]int64
}

type namedCacheKey struct {
	pool  string
	cache string
	os    string
}

func (n namedCacheKey) withoutOS() namedCacheKey {
	return namedCacheKey{pool: n.pool, cache: n.cache}
}

func newNamedCachesAggregatorShardState() *namedCachesAggregatorShardState {
	return &namedCachesAggregatorShardState{
		namedCacheMap: map[namedCacheKey][]int64{},
	}
}

func (s *namedCachesAggregatorShardState) collect(ctx context.Context, bot *model.BotInfo) {
	os := model.OSFamily(bot.DimensionsByKey("os"))
	pools := bot.DimensionsByKey("pool")
	if len(pools) == 0 {
		return
	}

	// Relevant bot state entry looks like this:
	//
	// "named_caches": {
	//   "git": [
	//     [
	//       "Zs",
	//       26848764602
	//     ],
	//     1708725132
	//   ],
	//   ...
	// }
	//
	// We are after the first number (i.e. 26848764602). Unfortunately this
	// structure can't be represented by a Go type, we'll have to use `any`.
	// To avoid losing precision, we also will have to ask JSON decoder to
	// unmarshal numbers as json.Number (instead of a float).
	stateEntry, err := bot.State.ReadRaw("named_caches")
	switch {
	case err != nil:
		logging.Warningf(ctx, "Bad named_caches for bot %q: %s", bot.BotID(), err)
		return
	case len(stateEntry) == 0:
		return
	}

	dec := json.NewDecoder(bytes.NewReader(stateEntry))
	dec.UseNumber()

	var namedCached map[string]any
	if err := dec.Decode(&namedCached); err != nil {
		logging.Warningf(ctx, "Bad named_caches for bot %q: %s", bot.BotID(), err)
		return
	}

	for cache, entry := range namedCached {
		if size, ok := extractCacheSize(entry); ok {
			for _, pool := range pools {
				key := namedCacheKey{
					pool:  pool,
					cache: cache,
					os:    os,
				}
				s.namedCacheMap[key] = append(s.namedCacheMap[key], size)
			}
		} else {
			logging.Warningf(ctx, "Bad named_caches entry for bot %q: %s", bot.BotID(), cache)
		}
	}
}

// extractCacheSize extracts cache size from "named_caches" state entry.
//
// It takes e.g. JSON-decoded `[["Zs", 26848764602], 1708725132]` and returns
// 26848764602.
func extractCacheSize(entry any) (int64, bool) {
	list, ok := entry.([]any)
	if !ok || len(list) != 2 {
		return 0, false
	}
	deeper, ok := list[0].([]any)
	if !ok || len(deeper) != 2 {
		return 0, false
	}
	size, ok := deeper[1].(json.Number)
	if !ok {
		return 0, false
	}
	if val, err := size.Int64(); err == nil {
		return val, true
	}
	return 0, false
}

// mergeFrom copies entries from `another` into `s`.
func (s *namedCachesAggregatorShardState) mergeFrom(another *namedCachesAggregatorShardState) {
	for key, sizes := range another.namedCacheMap {
		s.namedCacheMap[key] = append(s.namedCacheMap[key], sizes...)
	}
}

// updatePerOSEntries applies current values of per-OS cache size hints to
// an existing stored state `historic` and returns the resulting up-to-date
// state.
func updatePerOSEntries(now time.Time, current, historic []model.PerOSEntry) []model.PerOSEntry {
	// Throw away expired entries. Build a lookup map from what remains.
	remaining := make(map[string]model.PerOSEntry, len(historic))
	for _, entry := range historic {
		if entry.ExpireAt.After(now) {
			remaining[entry.Name] = entry
		}
	}

	// Calculate exponential moving average using existing entries as a baseline.
	for i := range current {
		if historic, ok := remaining[current[i].Name]; ok {
			current[i].Size = computeEMA(current[i].Size, historic.Size)
			delete(remaining, current[i].Name)
		}
	}

	// Carry over non-expired historic entries not present in `current`.
	for _, entry := range remaining {
		current = append(current, entry)
	}
	return current
}

// computeEMA computes the exponential moving average (EMA) of the current and
// previous EMA values.
//
// The NamedCachesAggregator runs every 10 min (see namedCachesUpdateFrequency).
// The smoothing factor is picked so that if an instantaneous named cache size
// measurement suddenly increases from 0% to 100%, the smoothed average will be
// at %63 after 1h. This matches the time scale of how fast we want to react to
// organic named cache size changes.
//
// See https://en.wikipedia.org/wiki/Exponential_smoothing#Time_constant.
//
//	periods = 1h / 10m = 6.0
//	alpha = 1.0 - math.exp(-1.0/periods) ~= 0.154
func computeEMA(current, previousEMA int64) int64 {
	if previousEMA == 0 {
		return current
	}
	return int64(float64(current-previousEMA)*0.154 + float64(previousEMA))
}
