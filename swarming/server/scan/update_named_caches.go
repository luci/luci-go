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
	// smoothingPeriod can be adjusted to influence the EMA.
	// Shorter period reacts more quickly to recent size changes,
	// while longer periods provide smoother trends.
	smoothingPeriod = 5
	// namedCachesExpiryTime is how long NamedCacheStats are kept in datastore.
	// Both NamedCacheStats entities and perOSEntries share the same expiry delay.
	// Datastore automatically cleans up expired entities in accordance its TTL policy.
	namedCachesExpiryTime = 8 * 24 * time.Hour
	// percentileThreshold is the percentile of the cache sizes to use as a hint.
	// 0.95 is a direct copy of the Python implementation.
	percentileThreshold = 0.95
	// batchSize represents the number of entities to process in a single datastore batch.
	batchSize = 500
)

// NamedCachesAggregator is a BotVisitor that collects
// all named_caches present on the bot and their respective size.
type NamedCachesAggregator struct {
	shards []*namedCachesAggregatorShardState
}

var _ BotVisitor = (*NamedCachesAggregator)(nil)

// Prepare prepares the visitor to use `shards` parallel queries.
//
// Part of BotVisitor interface.
func (a *NamedCachesAggregator) Prepare(ctx context.Context, shards int) {
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
	// Skip updating NamedCacheStats if the scan did not finish properly.
	if scanErr != nil {
		return nil
	}

	// Merge all shards together.
	merged := a.shards[0]
	for _, shard := range a.shards[1:] {
		merged.mergeFrom(shard)
	}

	// This cron job processes entities independently.
	// An error impacting 1 entity or 1 batch should not terminate the run.
	// Instead, collect the errors and return them as a MultiError at the end of the run.
	var gmerr errors.MultiError
	// Batch get all the relevant NamedCacheStats entities.
	toGet := make([]*model.NamedCacheStats, 0, len(merged.namedCacheMap))
	for id := range merged.namedCacheMap {
		if key := namedCacheStatsKeyFromStringID(ctx, id); key != nil {
			toGet = append(toGet, &model.NamedCacheStats{Key: key})
		}
	}

	// Process entities in batches sequentially to avoid
	// exceeding datastore limits and prevent out-of-memory errors.
	for len(toGet) > 0 {
		var dsmerr errors.MultiError
		var batch []*model.NamedCacheStats
		size := min(len(toGet), batchSize)
		batch, toGet = toGet[:size], toGet[size:]
		if err := datastore.Get(ctx, batch); err != nil && !errors.As(err, &dsmerr) {
			// `err` is not a MultiError in the case of an overall datastore failure (e.g. timed out).
			// Skip the problematic batch and store the error in a global MultiError.
			gmerr = append(gmerr, errors.Annotate(err, "fetching %d NamedCacheStats entities", len(batch)).Err())
			continue
		}

		// Save the updated NamedCacheStats entities from this batch.
		// Result is pushed to Datastore.
		toPut := make([]*model.NamedCacheStats, 0, len(batch))
		for index, ncs := range batch {
			// New NamedCacheStats are not yet stored in datastore. Ignore the `ErrNoSuchEntity` error in this case.
			if len(dsmerr) != 0 {
				if err := dsmerr[index]; err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
					gmerr = append(gmerr, errors.Annotate(err, "fetching NamedCacheStats entity: %s", ncs.Key.StringID()).Err())
					continue
				}
			}

			now := clock.Now(ctx)
			expiry := now.Add(namedCachesExpiryTime)
			// The key will always exist.
			osToSizesMap := merged.namedCacheMap[ncs.Key.StringID()]
			// Store all non-expired PerOSEntries for this entity.
			perOSEntryMap := make(map[string]model.PerOSEntry, len(osToSizesMap))

			for os, sizes := range osToSizesMap {
				// Calculate the cache sizes hint based on a percentile.
				slices.Sort(sizes)
				hint := sizes[int(float32(len(sizes))*percentileThreshold)]

				// Compute EMA only if there is a previous average.
				if index := perOSEntryIndex(os, ncs.OS); index != -1 {
					hint = computeEMA(hint, ncs.OS[index].Size)
				}

				perOSEntryMap[os] = model.PerOSEntry{
					Name:       os,
					Size:       hint,
					LastUpdate: now,
					ExpireAt:   expiry,
				}
			}

			// Add any non-expired PerOSEntries that were not updated by this cron run.
			for _, entry := range ncs.OS {
				if _, ok := perOSEntryMap[entry.Name]; !ok && entry.ExpireAt.After(now) {
					perOSEntryMap[entry.Name] = entry
				}
			}

			ncs.LastUpdate = now
			ncs.ExpireAt = expiry
			ncs.OS = perOSMapToSlice(perOSEntryMap)

			toPut = append(toPut, ncs)
		}

		if err := datastore.Put(ctx, toPut); err != nil {
			errors.Append(gmerr, errors.Annotate(err, "updating NamedCacheStats entities").Err())
		}
	}

	return gmerr.AsError()
}

////////////////////////////////////////////////////////////////////////////////////////////

type namedCachesAggregatorShardState struct {
	// The list of all observed cache sizes grouped by ("pool:cache") -> ("os") -> []size.
	namedCacheMap map[string]map[string][]int64
}

func newNamedCachesAggregatorShardState() *namedCachesAggregatorShardState {
	return &namedCachesAggregatorShardState{
		namedCacheMap: map[string]map[string][]int64{},
	}
}

func (s *namedCachesAggregatorShardState) collect(ctx context.Context, bot *model.BotInfo) {
	os := model.DetermineOSFamily(bot.DimensionsByKey("os"))
	pools := bot.DimensionsByKey("pool")
	// Discard bots unassigned to pools to be consistent with Python implementation.
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
		logging.Warningf(ctx, "bad state for bot %q: %s", bot.BotID(), err)
		return
	case len(stateEntry) == 0:
		return
	}

	dec := json.NewDecoder(bytes.NewReader(stateEntry))
	dec.UseNumber()

	var namedCached map[string]any
	if err := dec.Decode(&namedCached); err != nil {
		logging.Warningf(ctx, "bad state for bot %q: %s", bot.BotID(), err)
		return
	}

	for cache, entry := range namedCached {
		if size, ok := extractCacheSize(entry); ok {
			for _, pool := range pools {
				id := pool + ":" + cache
				if _, ok := s.namedCacheMap[id]; !ok {
					s.namedCacheMap[id] = map[string][]int64{}
				}
				oses := s.namedCacheMap[id]
				oses[os] = append(oses[os], size)
			}
		} else {
			logging.Warningf(ctx, "bad cache entry for bot %q: %s", bot.BotID(), cache)
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

func (s *namedCachesAggregatorShardState) mergeFrom(another *namedCachesAggregatorShardState) {
	for key, oses := range another.namedCacheMap {
		for os, sizes := range oses {
			if _, ok := s.namedCacheMap[key]; !ok {
				s.namedCacheMap[key] = map[string][]int64{}
			}
			s.namedCacheMap[key][os] = append(s.namedCacheMap[key][os], sizes...)
		}
	}
}

// perOSMapToSlice returns the values of osMap as a slice.
func perOSMapToSlice(osMap map[string]model.PerOSEntry) []model.PerOSEntry {
	entries := make([]model.PerOSEntry, 0, len(osMap))
	for _, entry := range osMap {
		entries = append(entries, entry)
	}
	return entries
}

// perOSEntryIndex returns the index of the perOSEntry with the given OS name.
// Returns -1 if not found.
func perOSEntryIndex(os string, entries []model.PerOSEntry) int {
	return slices.IndexFunc(entries, func(e model.PerOSEntry) bool {
		return e.Name == os
	})
}

// computeEMA computes the exponential moving average (EMA) of the current and
// previous EMA values. It uses a smoothing factor derived from smoothingPeriod.
func computeEMA(current int64, previousEMA int64) int64 {
	if previousEMA == 0 {
		return current
	}
	// Smoothing factor.
	alpha := 2 / (float64(smoothingPeriod + 1))
	return int64(float64((current-previousEMA))*alpha + float64(previousEMA))
}

// namedCacheStatsKeyFromStringID returns a NamedCacheStats key given its StringID.
// The format of id should be of the form "pool:cache".
func namedCacheStatsKeyFromStringID(ctx context.Context, id string) *datastore.Key {
	pool, cache, ok := strings.Cut(id, ":")
	if !ok {
		return nil
	}
	return model.NamedCacheStatsKey(ctx, pool, cache)
}
