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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// NamedCacheStats contains cache size hints for some (pool, cache) pair.
type NamedCacheStats struct {
	// Key identifies the pool and the cache, see NamedCacheStatsKey(...).
	Key *datastore.Key `gae:"$key"`
	// OS is per-OS entries with cache size hint for that OS.
	OS []PerOSEntry `gae:"os,noindex"`
	// LastUpdate is when this entity was updated the last time.
	LastUpdate time.Time `gae:"updated,noindex"`
	// ExpireAt is when this entity can be deleted, used in the TTL policy.
	ExpireAt time.Time `gae:"expiry,noindex"`
	// Extra are entity properties that didn't match any declared ones above.
	Extra datastore.PropertyMap `gae:"-,extra"`
}

// PerOSEntry contains the cache size hint for some concrete OS family.
type PerOSEntry struct {
	// Name is the OS family name, e.g. "Windows". See OSFamily(...).
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

// OSFamily returns the OS family given "os" dimension values.
func OSFamily(oses []string) string {
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

// FetchNamedCacheSizeHints fetches a statistical estimate of how many bytes
// given named caches are expected to occupy on the disk when the bot fully
// populates them.
//
// This is used by the bot to estimate if it has enough free disk space left
// to accommodate all requested named caches (and cleanup older caches if
// there's no enough free disk space).
//
// These stats are grouped by pool and OS family (i.e. the same named cache
// can have different size when used on different OSes or in different pools).
//
// On success the resulting slice has the same length as `caches`. Values are
// the corresponding named cache sizes in bytes, or -1 if the corresponding
// cache has never been seen before.
//
// All errors can be considered transient.
func FetchNamedCacheSizeHints(ctx context.Context, pool, osFamily string, caches []string) ([]int64, error) {
	ents := make([]*NamedCacheStats, len(caches))
	for i, cache := range caches {
		ents[i] = &NamedCacheStats{Key: NamedCacheStatsKey(ctx, pool, cache)}
	}

	var merr errors.MultiError
	if err := datastore.Get(ctx, ents); err != nil && !errors.As(err, &merr) {
		return nil, errors.Annotate(err, "fetching a batch of NamedCacheStats").Err()
	}

	out := make([]int64, len(caches))
	for i, ent := range ents {
		if len(merr) > 0 && merr[i] != nil && !errors.Is(merr[i], datastore.ErrNoSuchEntity) {
			return nil, errors.Annotate(merr[i], "fetching NamedCacheStats %s", ent.Key.StringID()).Err()
		}
		// Try to find the direct hit by the OS family. Otherwise pick the largest
		// value across all available OS families (if any) as a conservative
		// estimate. If ent.OS is empty (happens when the entity is missing), we
		// end up with -1, which indicates there's no such cache.
		var size int64 = -1
		for _, entry := range ent.OS {
			if entry.Name == osFamily {
				size = entry.Size
				break
			}
		}
		if size == -1 {
			for _, entry := range ent.OS {
				if entry.Size > size {
					size = entry.Size
				}
			}
		}
		out[i] = size
	}

	return out, nil
}
