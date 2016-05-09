// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/lru"
	"github.com/luci/luci-go/common/promise"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/metric"
	"golang.org/x/net/context"
)

const (
	// DefaultSize is the default (maximum) size of the LRU cache.
	DefaultSize = 1024 * 1024

	// DefaultExpiration is the default expiration value.
	DefaultExpiration = 10 * time.Minute
)

var (
	tsCache = metric.NewCounter("logdog/collector/coordinator/cache",
		"Metrics for cache uses, tracking hits and misses.",
		field.Bool("hit"))
)

// cache is a Coordinator interface implementation for the Collector service
// that caches remote results locally.
type cache struct {
	Coordinator

	// Size is the number of stream states to hold in the cache. If zero,
	// DefaultCacheSize will be used.
	size int

	// expiration is the maximum lifespan of a cache entry. If an entry is older
	// than this, it will be discarded. If zero, DefaultExpiration will be used.
	expiration time.Duration

	// cache is the LRU state cache.
	lru *lru.Cache
}

// NewCache creates a new Coordinator instance that wraps another Coordinator
// instance with a cache that retains the latest remote Coordiantor state in a
// client-side LRU cache.
func NewCache(c Coordinator, size int, expiration time.Duration) Coordinator {
	if size <= 0 {
		size = DefaultSize
	}
	if expiration <= 0 {
		expiration = DefaultExpiration
	}

	return &cache{
		Coordinator: c,
		expiration:  expiration,
		lru:         lru.New(size),
	}
}

// RegisterStream invokes the wrapped Coordinator's RegisterStream method and
// caches the result. It uses a Promise to cause all simultaneous identical
// RegisterStream requests to block on a single RPC.
func (c *cache) RegisterStream(ctx context.Context, st *LogStreamState, desc []byte) (*LogStreamState, error) {
	now := clock.Now(ctx)

	key := cacheEntryKey{
		project: st.Project,
		path:    st.Path,
	}

	// Get the cacheEntry from our cache. If it is expired, doesn't exist, or
	// we're forcing, ignore any existing entry and replace with a Promise pending
	// Coordinator sync.
	cacheHit := false
	entry := c.lru.Mutate(key, func(current interface{}) interface{} {
		// Don't replace an existing entry, unless it has an error or has expired.
		if current != nil {
			curEntry := current.(*cacheEntry)
			if !curEntry.hasError() && now.Before(curEntry.expiresAt) {
				cacheHit = true
				return current
			}
		}

		p := promise.New(func() (interface{}, error) {
			st, err := c.Coordinator.RegisterStream(ctx, st, desc)
			if err != nil {
				return nil, err
			}
			return st, nil
		})

		return &cacheEntry{
			cacheEntryKey: cacheEntryKey{
				project: st.Project,
				path:    st.Path,
			},
			terminalIndex: -1,
			p:             p,
			expiresAt:     now.Add(c.expiration),
		}
	}).(*cacheEntry)

	// If there was an error, purge the erroneous entry from the cache so that
	// the next "update" will re-fetch it.
	st, err := entry.get(ctx)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(ctx, "Error retrieving stream state.")
		return nil, err
	}

	tsCache.Add(ctx, 1, cacheHit)
	return st, nil
}

func (c *cache) TerminateStream(ctx context.Context, st *LogStreamState) error {
	key := cacheEntryKey{
		project: st.Project,
		path:    st.Path,
	}

	// Immediately update our state cache to record the terminal index, if
	// we have a state cache.
	c.lru.Mutate(key, func(current interface{}) (r interface{}) {
		// Always return the current entry. We're just atomically examining it to
		// load it with a terminal index.
		r = current
		if r != nil {
			r.(*cacheEntry).loadTerminalIndex(st.TerminalIndex)
		}
		return
	})

	return c.Coordinator.TerminateStream(ctx, st)
}

// cacheEntryKey is the LRU key for a cacheEntry.
type cacheEntryKey struct {
	project config.ProjectName
	path    types.StreamPath
}

// cacheEntry is the value stored in the cache. It contains a Promise
// representing the value and an optional invalidation singleton to ensure that
// if the state failed to fetch, it will be invalidated at most once.
//
// In addition to remote caching via Promise, the state can be updated locally
// by calling the cache's "put" method. In this case, the Promise will be nil,
// and the state value will be populated.
type cacheEntry struct {
	sync.Mutex
	cacheEntryKey

	// terminalIndex is the loaded terminal index set via loadTerminalIndex. It
	// will be applied to returned LogStreamState objects so that once a terminal
	// index has been set, we become aware of it in the Collector.
	terminalIndex types.MessageIndex

	// p is a Promise that is blocking pending a Coordiantor stream state
	// response. Upon successful resolution, it will contain a *LogStreamState.
	p promise.Promise

	project   config.ProjectName
	path      types.StreamPath
	expiresAt time.Time
}

// get returns the cached state that this entry owns, blocking until resolution
// if necessary.
//
// The returned state is a shallow copy of the cached state, and may be
// modified by the caller.
func (e *cacheEntry) get(ctx context.Context) (*LogStreamState, error) {
	promisedSt, err := e.p.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Create a clone of our cached value (not deep, so secret is not cloned, but
	// the Collector will not modify that). If we have a local terminal index
	// cached, apply that to the response.
	//
	// We need to lock around our terminalIndex.
	e.Lock()
	defer e.Unlock()

	rp := *(promisedSt.(*LogStreamState))
	if rp.TerminalIndex < 0 {
		rp.TerminalIndex = e.terminalIndex
	}

	return &rp, nil
}

// hasError tests if this entry has completed evaluation with an error state.
// This is non-blocking, so if the evaluation hasn't completed, it will return
// false.
func (e *cacheEntry) hasError() bool {
	if _, err := e.p.Peek(); err != nil && err != promise.ErrNoData {
		return true
	}
	return false
}

// loadTerminalIndex loads a local cache of the stream's terminal index. This
// will be applied to all future get requests.
func (e *cacheEntry) loadTerminalIndex(tidx types.MessageIndex) {
	e.Lock()
	defer e.Unlock()

	e.terminalIndex = tidx
}
