// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package coordinatorTest

import (
	"sync"
	"time"

	"github.com/luci/luci-go/logdog/common/storage/caching"

	"golang.org/x/net/context"
)

// StorageCache wraps the default storage cache instance, adding tracking
// information that is useful for testing.
type StorageCache struct {
	Base caching.Cache

	statsMu sync.Mutex
	stats   StorageCacheStats
}

// StorageCacheStats expose stats for a StorageCache instance.
type StorageCacheStats struct {
	Puts   int
	Hits   int
	Misses int
}

// Clear clears the stats for this cache instance.
func (sc *StorageCache) Clear() {
	sc.statsMu.Lock()
	defer sc.statsMu.Unlock()
	sc.stats = StorageCacheStats{}
}

// Stats returns the stats for this cache instance.
func (sc *StorageCache) Stats() StorageCacheStats {
	sc.statsMu.Lock()
	defer sc.statsMu.Unlock()
	return sc.stats
}

// Get implements caching.Cache.
func (sc *StorageCache) Get(c context.Context, items ...*caching.Item) {
	sc.Base.Get(c, items...)

	// Calculate our hit/misses from the result.
	sc.statsMu.Lock()
	defer sc.statsMu.Unlock()
	for _, it := range items {
		if it.Data != nil {
			sc.stats.Hits++
		} else {
			sc.stats.Misses++
		}
	}

}

// Put implements caching.Cache.
func (sc *StorageCache) Put(c context.Context, exp time.Duration, items ...*caching.Item) {
	sc.Base.Put(c, exp, items...)

	sc.statsMu.Lock()
	defer sc.statsMu.Unlock()
	sc.stats.Puts += len(items)
}
