// Copyright 2016 The LUCI Authors.
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

package coordinatorTest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/gcloud/gs"

	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/archive"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
)

// BigTableStorage is a bound BigTable-backed Storage instance.
type BigTableStorage struct {
	// Testing is the base in-memory BigTable instance that is backing this
	// Storage.
	bigtable.Testing

	closed int32
}

// GetSignedURLs implements coordinator.Storage.
func (st *BigTableStorage) GetSignedURLs(context.Context, *coordinator.URLSigningRequest) (
	*coordinator.URLSigningResponse, error) {
	return nil, nil
}

// Close implements storage.Storage.
func (st *BigTableStorage) Close() {
	if atomic.AddInt32(&st.closed, 1) != 1 {
		panic("already closed")
	}
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

// GetSignedURLs implements coordinator.SigningStorage.
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
	Base storage.Cache

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

// Get implements storage.Cache.
func (sc *StorageCache) Get(c context.Context, key storage.CacheKey) ([]byte, bool) {
	v, ok := sc.Base.Get(c, key)

	// Calculate our hit/misses from the result.
	sc.statsMu.Lock()
	defer sc.statsMu.Unlock()
	if ok {
		sc.stats.Hits++
	} else {
		sc.stats.Misses++
	}

	return v, ok
}

// Put implements storage.Cache.
func (sc *StorageCache) Put(c context.Context, key storage.CacheKey, v []byte, exp time.Duration) {
	sc.Base.Put(c, key, v, exp)

	sc.statsMu.Lock()
	defer sc.statsMu.Unlock()
	sc.stats.Puts++
}
