// Copyright 2023 The LUCI Authors.
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

package cfg

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// Provider knows how to load preprocessed configs from the datastore and
// convert them into instances of Config.
//
// Provider is a long-living object that keeps a reference to the most recent
// config, periodically reloading it from the datastore.
type Provider struct {
	cur atomic.Value
	m   sync.Mutex

	// These define how often VersionInfo field of a semantically unchanging
	// config is updated (see Latest implementation). Replaced with smaller values
	// in tests.
	versionInfoRefreshMinAge time.Duration
	versionInfoRefreshMaxAge time.Duration
}

// NewProvider initializes a Provider by fetching the initial copy of configs.
//
// If there are no configs stored in the datastore (happens when bootstrapping
// a new service), uses some default empty config.
func NewProvider(ctx context.Context) (*Provider, error) {
	cur, err := fetchFromDatastore(ctx)
	if err != nil {
		return nil, err
	}
	p := &Provider{
		versionInfoRefreshMinAge: 5 * time.Second,
		versionInfoRefreshMaxAge: 10 * time.Second,
	}
	p.cur.Store(cur)
	return p, nil
}

// Cached returns an immutable snapshot of the currently cached config.
//
// Returns the config cached in the local process memory. It may be slightly
// behind the most recently ingested config. Use Latest() to bypass the cache
// and get the most recently ingested config instead.
//
// Note that multiple sequential calls to Cached() may return different
// snapshots if the config changes between them. For that reason it is better to
// get the snapshot once at the beginning of an RPC handler, and use it
// throughout.
func (p *Provider) Cached(ctx context.Context) *Config {
	return p.cur.Load().(*Config)
}

// Latest returns an immutable snapshot of the most recently ingested config by
// fetching it from the datastore.
//
// Can be used in places where staleness of the cached config is unacceptable.
//
// Updates the local cache as a side effect.
func (p *Provider) Latest(ctx context.Context) (*Config, error) {
	// Get the most recent config digest in the datastore right now.
	latest, err := fetchConfigBundleRev(ctx)
	if err != nil {
		return nil, err
	}

	// Check if we actually already loaded this config (or even something newer,
	// in case some other goroutine called Latest concurrently).
	cur := p.cur.Load().(*Config)
	switch {
	case cur.VersionInfo.Fetched.After(latest.Fetched):
		// Some other goroutine already fetched some newer config, use it.
		return cur, nil
	case cur.VersionInfo.Fetched.Equal(latest.Fetched):
		// Already have the exact same config (semantically). We still need to
		// update not semantically meaningful fields in it (like the git revision:
		// they all are in VersionInfo). Do it only occasionally to avoid contention
		// on concurrently updating `p.cur` atomic all the time (use random jitter
		// to desynchronize requests). Note that staleness of these fields should
		// not matter. They are FYI only, and not involved when calculating the
		// config digest. They mostly show up in logs.
		age := clock.Since(ctx, cur.Refreshed)
		jitter := time.Duration(rand.Int63n(int64(p.versionInfoRefreshMaxAge - p.versionInfoRefreshMinAge)))
		if age > p.versionInfoRefreshMinAge+jitter {
			// The lock is needed to avoid messing with the config reload that might
			// already be happening concurrently (see its code below).
			//
			// In particular without the lock, it is possible this code section is
			// executing in between clock.Now(...) in fetchFromDatastore and
			// p.cur.Store(...). As a result Refreshed can briefly be seen going
			// backwards in time:
			//  1. clock.Now(...) in fetchFromDatastore reads T1.
			//  2. This block runs and updates Refreshed to some T2 (T2 > T1).
			//  3. This is observed by someone, they see Refreshed == T2.
			//  4. p.cur.Store(...) in the config reload updates Refreshed to T1.
			//  5. The observer sees T1 now, after already seeing T2 => violation.
			//
			// Note that we don't need to do any more checks beyond just getting the
			// lock: if `p.cur` changed by the config reloader after we've got the
			// lock, the CompareAndSwap will just do nothing.
			//
			// An alternative is to always use CompareAndSwap instead of Store when
			// updating the atomic value (including in the config reloader code path)
			// to guarantee Refreshed never goes back, but it ends up being more
			// complicated.
			p.m.Lock()
			defer p.m.Unlock()
			clone := *cur
			clone.VersionInfo = latest.VersionInfo
			clone.Refreshed = clock.Now(ctx)
			p.cur.CompareAndSwap(cur, &clone)
			return p.cur.Load().(*Config), nil
		}
		return cur, nil
	}

	// Avoid hitting the expensive code path concurrently. There is a stampede
	// of concurrent requests here when the config actually changes. Let them all
	// wait for the first request that grabs the lock to do the expensive fetch.
	// Once the fetch is done, other requests will grab the lock one by one and
	// quickly realize there's nothing to do.
	p.m.Lock()
	defer p.m.Unlock()

	// Recheck the current config under the lock. Maybe some other goroutine
	// already loaded a newer one.
	cur = p.cur.Load().(*Config)
	if !latest.Fetched.After(cur.VersionInfo.Fetched) {
		return cur, nil
	}

	// Load the latest config from the datastore (it may be more recent that
	// `latest` at that point).
	logging.Infof(ctx, "Reloading config...")
	new, err := fetchFromDatastore(ctx)
	if err != nil {
		return nil, err
	}
	p.cur.Store(new)
	return new, nil
}

// FreshEnough returns an immutable snapshot of a config either by using the
// local process cache (if it is fresh enough) or by fetching the latest
// config from the datastore.
//
// `seen` should be a VersionInfo.Fetched value from some previously seen
// config version.
//
// If the VersionInfo.Fetched of the currently cached config is equal or
// larger than `seen`, returns the cached config. Otherwise fetches the latest
// config (its VersionInfo.Fetched will be larger or equal than `seen`).
//
// This is used to avoid going back to a previous config version once a process
// seen a newer config version at least once.
func (p *Provider) FreshEnough(ctx context.Context, seen time.Time) (*Config, error) {
	cur := p.cur.Load().(*Config)
	if seen.After(cur.VersionInfo.Fetched) {
		// There's a newer config out there, since some process seen it already.
		return p.Latest(ctx)
	}
	return cur, nil
}

// RefreshPeriodically runs a loop that periodically refetches the config from
// the datastore to update the local cache.
func (p *Provider) RefreshPeriodically(ctx context.Context) {
	for {
		jitter := time.Duration(rand.Int63n(int64(10 * time.Second)))
		if r := <-clock.After(ctx, 30*time.Second+jitter); r.Err != nil {
			return // the context is canceled
		}
		if _, err := p.Latest(ctx); err != nil {
			// Don't log the error if the server is shutting down.
			if !errors.Is(err, context.Canceled) {
				logging.Warningf(ctx, "Failed to refresh local copy of the config: %s", err)
			}
		}
	}
}
