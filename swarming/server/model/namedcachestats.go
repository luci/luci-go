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
	"slices"
	"time"

	"go.chromium.org/luci/gae/service/datastore"
)

// NamedCacheStats contains cache size hints for some (pool, cache) pair.
// Datastore automatically deletes expired NamedCacheStats entities as per the TTL policy.
type NamedCacheStats struct {
	// Key identifies the pool and the cache, see NamedCacheStatsKey(...).
	Key *datastore.Key `gae:"$key"`
	// OS is per-OS entries with cache size hint for that OS.
	OS []PerOSEntry `gae:"os,noindex"`
	// ExpireAt is when this entity can be deleted.
	ExpireAt time.Time `gae:"expiry,noindex"`
	// LastUpdate is when this entity was updated the last time.
	LastUpdate time.Time `gae:"updated,noindex"`
	// Extra are entity properties that didn't match any declared ones above.
	Extra datastore.PropertyMap `gae:"-,extra"`
}

// PerOSEntry contains the cache size hint for some concrete OS family.
type PerOSEntry struct {
	// Name is the OS family name, e.g. "Windows". See DetermineOSFamily(...).
	Name string `gae:"name,noindex"`
	// Size is the current estimate of the cache size for this OS in bytes.
	Size int64 `gae:"size,noindex"`
	// LastUpdate is when this entry was updated the last time.
	LastUpdate time.Time `gae:"updated,noindex"`
	// ExpireAt is when this entry can be deleted.
	ExpireAt time.Time `gae:"expiry,noindex"`
}

// NamedCacheStatsKey returns a NamedCacheStats key given a cache and a pool.
func NamedCacheStatsKey(ctx context.Context, pool, cache string) *datastore.Key {
	return datastore.NewKey(ctx, "NamedCacheStats", pool+":"+cache, 0, nil)
}

// DetermineOSFamily returns the OS family given "os" dimension values.
func DetermineOSFamily(oses []string) string {
	if len(oses) == 0 {
		return "Unknown"
	}
	for _, known := range []string{"Windows", "Mac", "Android", "Linux"} {
		if slices.Contains(oses, known) {
			return known
		}
	}
	return slices.Min(oses)
}
