// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package coordinatorTest

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/logdog/appengine/coordinator"
	"github.com/luci/luci-go/logdog/common/storage"
	"github.com/luci/luci-go/logdog/common/storage/archive"
	"github.com/luci/luci-go/logdog/common/storage/bigtable"
	"github.com/luci/luci-go/logdog/common/storage/caching"

	"golang.org/x/net/context"
)

// BigTableStorage is a bound BigTable-backed Storage instance.
type BigTableStorage struct {
	// Testing is the base in-memory BigTable instance that is backing this
	// Storage.
	bigtable.Testing

	closed int32
}

// GetSignedURLs implements coordinator.Storage.
func (st *BigTableStorage) GetSignedURLs(context.Context, *coordinator.URLSigningRequest) (*coordinator.URLSigningResponse, error) {
	return nil, nil
}

// Close implements storage.Storage.
func (st *BigTableStorage) Close() {
	if atomic.AddInt32(&st.closed, 1) != 1 {
		panic("already closed")
	}
}

// Config implements storage.Storage.
func (st *BigTableStorage) Config(storage.Config) error {
	return errors.New("not implemented")
}

// ArchivalStorage is a bound GSClient-backed Storage instance.
type ArchivalStorage struct {
	// Storage is the base (archive) Storage instance.
	storage.Storage

	// Opts is the set of archival options that the Storage was instantiated with.
	Opts archive.Options

	closed int32
}

// Close implements storage.Storage.
func (st *ArchivalStorage) Close() {
	if atomic.AddInt32(&st.closed, 1) != 1 {
		panic("already closed")
	}
	st.Storage.Close()
}

// GetSignedURLs implements coordinator.Storage.
func (st *ArchivalStorage) GetSignedURLs(c context.Context, req *coordinator.URLSigningRequest) (
	*coordinator.URLSigningResponse, error) {

	if req.Lifetime < 0 {
		return nil, errors.New("invalid lifetime")
	}

	resp := coordinator.URLSigningResponse{
		Expiration: clock.Now(c).Add(req.Lifetime),
	}

	signURL := func(s gs.Path) string { return string(s) + "&signed=true" }
	if req.Stream {
		resp.Stream = signURL(st.Opts.Stream)
	}
	if req.Index {
		resp.Index = signURL(st.Opts.Index)
	}
	return &resp, nil
}

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
