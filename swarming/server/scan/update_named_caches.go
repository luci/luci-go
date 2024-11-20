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

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/swarming/server/model"
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

	// TODO: Finish implementation.

	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////

type namedCachesAggregatorShardState struct {
	// The list of all observed cache sizes grouped by ("cache:pool") -> ("os") -> []size.
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
	var botState struct {
		NamedCaches map[string]any `json:"named_caches"`
	}
	dec := json.NewDecoder(bytes.NewReader(bot.State))
	dec.UseNumber()
	if err := dec.Decode(&botState); err != nil {
		logging.Warningf(ctx, "bad state for bot %q: %s", bot.BotID(), err)
		return
	}
	for cache, entry := range botState.NamedCaches {
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
