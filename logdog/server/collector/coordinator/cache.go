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

package coordinator

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/data/caching/lru"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/promise"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"

	"go.chromium.org/luci/logdog/common/types"
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
		nil,
		field.Bool("hit"))
)

// cache is a Coordinator interface implementation for the Collector service
// that caches remote results locally.
type cache struct {
	Coordinator

	// expiration is the maximum lifespan of a cache entry. If an entry is older
	// than this, it will be discarded.
	expiration time.Duration

	// cache is the LRU state cache.
	lru *lru.Cache[cacheEntryKey, *cacheEntry]
}

// NewCache creates a new Coordinator instance that wraps another Coordinator
// instance with a cache that retains the latest remote Coordinator state in a
// client-side LRU cache.
//
// If size is <= 0, DefaultSize will be used.
// If expiration is <= 0, DefaultExpiration will be used.
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
		lru:         lru.New[cacheEntryKey, *cacheEntry](size),
	}
}

func (c *cache) getCacheEntry(ctx context.Context, k cacheEntryKey) (*cacheEntry, bool) {
	// Get the cacheEntry from our cache. If it is expired or doesn't exist,
	// generate a new cache entry for this key.
	created := false
	entry, _ := c.lru.Mutate(ctx, k, func(it *lru.Item[*cacheEntry]) *lru.Item[*cacheEntry] {
		// Don't replace an existing entry, unless it has an error or has expired.
		if it != nil {
			return it
		}

		created = true
		return &lru.Item[*cacheEntry]{
			Value: &cacheEntry{
				cacheEntryKey: k,
				terminalIndex: -1,
			},
			Exp: c.expiration,
		}
	})
	return entry, created
}

// RegisterStream invokes the wrapped Coordinator's RegisterStream method and
// caches the result. It uses a Promise to cause all simultaneous identical
// RegisterStream requests to block on a single RPC.
func (c *cache) RegisterStream(ctx context.Context, st *LogStreamState, desc []byte) (*LogStreamState, error) {
	entry, created := c.getCacheEntry(ctx, cacheEntryKey{
		project: st.Project,
		path:    st.Path,
	})
	tsCache.Add(ctx, 1, !created)

	st, err := entry.registerStream(ctx, c.Coordinator, *st, desc)
	if err != nil {
		log.WithError(err).Errorf(ctx, "Error retrieving stream state.")
		return nil, err
	}

	return st, nil
}

func (c *cache) TerminateStream(ctx context.Context, r *TerminateRequest) error {
	entry, _ := c.getCacheEntry(ctx, cacheEntryKey{
		project: r.Project,
		path:    r.Path,
	})
	return entry.terminateStream(ctx, c.Coordinator, *r)
}

// cacheEntryKey is the LRU key for a cacheEntry.
type cacheEntryKey struct {
	project string
	path    types.StreamPath
}

// cacheEntry is a cached state for a specific log stream.
//
// It contains promises for each singleton operation: one for stream
// registration (registerP), and one for stream termination (terminateP).
//
// There are three states to promise evaluation:
//   - If the promise is nil, it will be populated. Any concurrent requests
//     will block pending population (via lock) and will obtain a reference to
//     the populated promise.
//   - If the promise succeeded, or failed non-transiently, its result will be
//     retained and all subsequent calls will see this result.
//   - If the promise failed transiently, it will be set to nil. This will cause
//     the next caller to generate a new promise (retry). Concurrent users of the
//     transiently-failing Promise will all receive a transient error.
type cacheEntry struct {
	sync.Mutex
	cacheEntryKey

	// terminalIndex is the cached terminal index. Valid if >= 0.
	//
	// If a TerminateStream RPC succeeds, we will use this value in our returned
	// RegisterStream state.
	terminalIndex types.MessageIndex

	// registerP is a Promise that is blocking pending stream registration.
	// Upon successful resolution, it will contain a *LogStreamState.
	registerP *promise.Promise[any]
	// terminateP is a Promise that is blocking pending stream termination.
	// Upon successful resolution, it will contain a nil result with no error.
	terminateP *promise.Promise[any]
}

// registerStream performs a RegisterStream Coordinator RPC.
func (ce *cacheEntry) registerStream(ctx context.Context, coord Coordinator, st LogStreamState, desc []byte) (*LogStreamState, error) {
	// Initialize the registration Promise, if one is not defined.
	//
	// While locked, load the current registration promise and the local
	// terminal index value.
	var (
		p    *promise.Promise[any]
		tidx types.MessageIndex
	)
	ce.withLock(func() {
		if ce.registerP == nil {
			ce.registerP = promise.NewDeferred(func(ctx context.Context) (any, error) {
				st, err := coord.RegisterStream(ctx, &st, desc)
				if err == nil {
					// If the remote state has a terminal index, retain it locally.
					ce.loadRemoteTerminalIndex(st.TerminalIndex)
					return st, nil
				}

				return nil, err
			})
		}

		p, tidx = ce.registerP, ce.terminalIndex
	})

	// Resolve our registration Promise.
	remoteStateIface, err := p.Get(ctx)
	if err != nil {
		// If the promise failed transiently, clear it so that subsequent callers
		// will regenerate a new promise. ONLY clear it if it it is the same
		// promise, as different callers may have already cleared/rengerated it.
		if transient.Tag.In(err) {
			ce.withLock(func() {
				if ce.registerP == p {
					ce.registerP = nil
				}
			})
		}
		return nil, err
	}
	remoteState := remoteStateIface.(*LogStreamState)

	// If our remote state doesn't include a terminal index and our local state
	// has recorded a successful remote terminal index, return a copy of the
	// remote state with the remote terminal index added.
	if remoteState.TerminalIndex < 0 && tidx >= 0 {
		remoteStateCopy := *remoteState
		remoteStateCopy.TerminalIndex = tidx
		remoteState = &remoteStateCopy
	}
	return remoteState, nil
}

// terminateStream performs a TerminateStream Coordinator RPC.
func (ce *cacheEntry) terminateStream(ctx context.Context, coord Coordinator, tr TerminateRequest) error {
	// Initialize the termination Promise if one is not defined. Also, grab our
	// cached remote terminal index.
	var (
		p    *promise.Promise[any]
		tidx types.MessageIndex = -1
	)
	ce.withLock(func() {
		if ce.terminateP == nil {
			// We're creating a new promise, so our tr's TerminalIndex will be set.
			ce.terminateP = promise.NewDeferred(func(ctx context.Context) (any, error) {
				// Execute our TerminateStream RPC. If successful, retain the successful
				// terminal index locally.
				err := coord.TerminateStream(ctx, &tr)
				if err == nil {
					// Note that this happens within the Promise body, so this will not
					// conflict with our outer lock.
					ce.loadRemoteTerminalIndex(tr.TerminalIndex)
				}
				return nil, err
			})
		}

		p, tidx = ce.terminateP, ce.terminalIndex
	})

	// If the stream is known to be terminated on the Coordinator side, we don't
	// need to issue another request.
	if tidx >= 0 {
		if tr.TerminalIndex != tidx {
			// Not much we can do here, and this probably will never happen, but let's
			// log it if it does.
			log.Fields{
				"requestIndex": tr.TerminalIndex,
				"cachedIndex":  tidx,
			}.Warningf(ctx, "Request terminal index doesn't match cached value.")
		}
		return nil
	}

	// Resolve our termination Promise.
	if _, err := p.Get(ctx); err != nil {
		// If this is a transient error, delete this Promise so future termination
		// attempts will retry for this stream.
		if transient.Tag.In(err) {
			ce.withLock(func() {
				if ce.terminateP == p {
					ce.terminateP = nil
				}
			})
		}
		return err
	}
	return nil
}

// loadRemoteTerminalIndex updates our cached remote terminal index with tidx,
// if tidx >= 0 and a remote terminal index is not already cached.
//
// This is executed in the bodies of the register and terminate Promises if they
// receive a terminal index remotely.
func (ce *cacheEntry) loadRemoteTerminalIndex(tidx types.MessageIndex) {
	// Never load an invalid remote terminal index.
	if tidx < 0 {
		return
	}

	// Load the remote terminal index if one isn't already loaded.
	ce.withLock(func() {
		if ce.terminalIndex < 0 {
			ce.terminalIndex = tidx
		}
	})
}

func (ce *cacheEntry) withLock(f func()) {
	ce.Lock()
	defer ce.Unlock()
	f()
}
