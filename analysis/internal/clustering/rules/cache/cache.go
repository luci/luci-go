// Copyright 2022 The LUCI Authors.
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

package cache

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/server/caching"
)

// refreshInterval controls how often rulesets are refreshed.
const refreshInterval = time.Minute

// StrongRead is a special time used to request the read of a ruleset
// that contains all rule changes committed prior to the start of the
// read. (Rule changes made after the start of the read may also
// be returned.)
// Under the covers, this results in a Spanner Strong Read.
// See https://cloud.google.com/spanner/docs/reads for more.
var StrongRead = time.Unix(0, 0).In(time.FixedZone("RuleCache StrongRead", 0xDB))

// RulesCache is an in-process cache of failure association rules used
// by LUCI projects.
type RulesCache struct {
	cache caching.LRUHandle[string, *Ruleset]
}

// NewRulesCache initialises a new RulesCache.
func NewRulesCache(c caching.LRUHandle[string, *Ruleset]) *RulesCache {
	return &RulesCache{
		cache: c,
	}
}

// Ruleset obtains the Ruleset for a particular project from the cache, or if
// it does not exist, retrieves it from Spanner. MinimumPredicatesVersion
// specifies the minimum version of rule predicates that must be incorporated
// in the given Ruleset. If no particular version is desired, pass
// rules.StartingEpoch. If a strong read is required, pass StrongRead.
// Otherwise, pass the particular (minimum) version required.
func (c *RulesCache) Ruleset(ctx context.Context, project string, minimumPredicatesVersion time.Time) (*Ruleset, error) {
	var err error
	readStart := clock.Now(ctx)

	// Fast path: try and use the existing cached value (if any).
	ruleset, ok := c.cache.LRU(ctx).Get(ctx, project)
	if ok {
		if isRulesetUpToDate(ruleset, readStart, minimumPredicatesVersion) {
			return ruleset, nil
		}
	}

	// Update the cache. This requires acquiring the mutex that
	// controls updates to the cache entry.
	ruleset, _ = c.cache.LRU(ctx).Mutate(ctx, project, func(it *lru.Item[*Ruleset]) *lru.Item[*Ruleset] {
		// Only one goroutine will enter this section at one time.
		var ruleset *Ruleset
		if it != nil {
			ruleset = it.Value
			if isRulesetUpToDate(ruleset, readStart, minimumPredicatesVersion) {
				// The ruleset is up-to-date. Do not mutate it further.
				// This can happen if the ruleset updated while we were
				// waiting to acquire the mutex to update the cache entry.
				return it
			}
		} else {
			ruleset = newEmptyRuleset(project)
		}
		ruleset, err = ruleset.refresh(ctx)
		if err != nil {
			// Issue refreshing ruleset. Keep the cached value (if any) for now.
			return it
		}
		return &lru.Item[*Ruleset]{
			Value: ruleset,
			Exp:   0, // Never.
		}
	})
	if err != nil {
		return nil, err
	}
	if minimumPredicatesVersion != StrongRead && ruleset.Version.Predicates.Before(minimumPredicatesVersion) {
		return nil, fmt.Errorf("could not obtain ruleset of requested minimum predicate version (%v)", minimumPredicatesVersion)
	}
	return ruleset, nil
}

func isRulesetUpToDate(rs *Ruleset, readStart, minimumPredicatesVersion time.Time) bool {
	if minimumPredicatesVersion == StrongRead {
		if rs.LastRefresh.After(readStart) {
			// We deliberately use a cached ruleset for some strong
			// reads so long as the refresh occurred after the call to
			// Ruleset(...).
			// This is to ensure that even if Ruleset(...) receives
			// many requests for StrongReads, each will at most need
			// to wait for the next strong read to complete, rather
			// than being bottlenecked by the fact only one goroutine
			// can enter the section to update the cache entry at once.
			return true
		}
	} else {
		if rs.LastRefresh.Add(refreshInterval).After(readStart) && !rs.Version.Predicates.Before(minimumPredicatesVersion) {
			return true
		}
	}
	return false
}
