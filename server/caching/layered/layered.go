// Copyright 2017 The LUCI Authors.
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

// Package layered provides a two-layer cache for serializable objects.
package layered

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/caching"
)

// ErrCantSatisfyMinTTL is returned by GetOrCreate if the factory function
// produces an item that expires sooner than the requested MinTTL.
var ErrCantSatisfyMinTTL = errors.New("new item produced by the factory has insufficient TTL")

// Cache implements a cache of serializable objects on top of process and
// global caches.
//
// If the global cache is not available or fails, degrades to using only process
// cache.
//
// Since global cache errors are ignored, gives no guarantees of consistency or
// item uniqueness. Thus supposed to be used only when caching results of
// computations without side effects.
type Cache struct {
	// ProcessLRUCache is a handle to a process LRU cache that holds the data.
	ProcessLRUCache caching.LRUHandle
	// GlobalNamespace is a global cache namespace to use for the data.
	GlobalNamespace string
	// Marshal converts an item being cached to a byte blob.
	Marshal func(item interface{}) ([]byte, error)
	// Unmarshal takes output of Marshal and converts it to an item to return.
	Unmarshal func(blob []byte) (interface{}, error)
}

// Option is a base interface of options for GetOrCreate call.
type Option interface {
	apply(opts *options)
}

// WithMinTTL specifies minimal acceptable TTL (Time To Live) of the returned
// cached item.
//
// If the currently cached item expires sooner than the requested TTL, it will
// be forcefully refreshed. If the new (refreshed) item also expires sooner
// than the requested min TTL, GetOrCreate will return ErrCantSatisfyMinTTL.
func WithMinTTL(ttl time.Duration) Option {
	if ttl <= 0 {
		panic("ttl must be positive")
	}
	return minTTLOpt(ttl)
}

// WithRandomizedExpiration enables randomized early expiration.
//
// This is only useful if cached items are used highly concurrently from many
// goroutines.
//
// On each cache access if the remaining TTL of the cached item is less than
// 'threshold', it may randomly be considered already expired (with probability
// increasing when item nears its true expiration).
//
// This is useful to avoid a situation when many concurrent consumers discover
// at the same time that the item has expired, and then all proceed waiting
// for a refresh. With randomized early expiration only the most unlucky
// consumer will trigger the refresh and will be blocked on it.
func WithRandomizedExpiration(threshold time.Duration) Option {
	if threshold < 0 {
		panic("threshold must be positive")
	}
	return expRandThresholdOpt(threshold)
}

// WithAllowedStaleness indicates that it is OK to return a stale item if
// the refresh procedure fails with a transient error.
//
// This option may be used to improve resiliency in cases when the cached data
// is fetched from another service and this service goes down. In many cases
// it is acceptable to keep using stale cached data instead of evicting it from
// the cache and failing on fetching the fresh one.
//
// Note that items created with this option never actually expire in the
// global cache, so it should be used carefully if the cordiality of the set of
// all possible items is huge. Note that items still may be evicted due to
// memory pressure or if the cache is flushed.
func WithAllowedStaleness() Option {
	return allowedStalenessOpt{}
}

// GetOrCreate attempts to grab an item from process or global cache, or create
// it if it's not cached yet.
//
// Fetching an item from the global cache or instantiating a new item happens
// under a per-key lock.
//
// Expiration time is used with seconds precision. Zero expiration time means
// the item doesn't expire on its own.
func (c *Cache) GetOrCreate(ctx context.Context, key string, fn lru.Maker, opts ...Option) (interface{}, error) {
	if c.GlobalNamespace == "" {
		panic("empty namespace is forbidden, please specify GlobalNamespace")
	}

	o := options{}
	for _, opt := range opts {
		opt.apply(&o)
	}

	now := clock.Now(ctx)
	lru := c.ProcessLRUCache.LRU(ctx)

	// Check that the item is in the local cache, its TTL is acceptable and we
	// don't want to randomly prematurely expire it, see WithRandomizedExpiration.
	var ignored *itemWithExp
	if v, ok := lru.Get(ctx, key); ok {
		item := v.(*itemWithExp)
		if item.isAcceptableTTL(now, o.minTTL) && !item.randomlyExpired(ctx, now, o.expRandThreshold) {
			return item.val, nil
		}
		ignored = item
	}

	// Either the item is not in the local cache, or the cached copy expires too
	// soon or we randomly decided that we want to prematurely refresh it. Attempt
	// to fetch from the global cache or create a new one. Disable expiration
	// randomization at this point, it has served its purpose already, since only
	// unlucky callers will reach this code path.
	v, err := lru.Create(ctx, key, func() (interface{}, time.Duration, error) {
		// Now that we have the lock, recheck that the item still needs a refresh.
		// Purposely ignore an item we decided we want to prematurely expire.
		var local *itemWithExp
		if v, ok := lru.Get(ctx, key); ok {
			if local = v.(*itemWithExp); local != ignored && local.isAcceptableTTL(now, o.minTTL) {
				return local, local.expiration(now), nil
			}
		}

		// Here 'local' is either nil or expired. Attempt to grab a potentially
		// newer copy from the global cache, maybe someone else already updated it.
		var global *itemWithExp
		if global = c.maybeFetchItem(ctx, key); global != nil && global.isAcceptableTTL(now, o.minTTL) {
			return global, global.expiration(now), nil
		}

		// Either a cache miss, deserialization errors or the TTL is not acceptable.
		// Need to make a new item.
		var item itemWithExp
		val, exp, err := fn()
		item.val = val
		switch {
		case transient.Tag.In(err) && o.allowedStaleness:
			if reused := tryReuseStale(ctx, now, &o, local, global); reused != nil {
				logging.WithError(err).Errorf(ctx,
					"Failed to refresh item %q (attempt %d, will try again in %s), reusing the stale one (expiry %s)",
					key, reused.retries, reused.expiration(now), reused.origExp)
				return reused, reused.expiration(now), nil
			}
			return nil, 0, err
		case err != nil:
			return nil, 0, err
		case exp < 0: // bad API usage
			panic("the expiration time must be non-negative")
		case exp == 0: // indicates the item never expires
			item.exp = time.Time{}
		case exp > 0:
			item.exp = now.Add(exp)
			if !item.isAcceptableTTL(now, o.minTTL) {
				// If 'fn' is incapable of generating an item with sufficient TTL
				// there's nothing else we can do.
				return nil, 0, ErrCantSatisfyMinTTL
			}
		}

		// When using WithAllowedStaleness we don't actually ever want to evict
		// items from the global cache (since they are useful even when stale).
		if o.allowedStaleness {
			exp = 0
		}

		// Store the new item in the global cache. We may accidentally override
		// it there if someone else refreshed it already. But this is unavoidable
		// given GlobalCache semantics and generally rare and harmless (given Cache
		// guarantees or rather lack of there of).
		if err := c.maybeStoreItem(ctx, key, &item, exp); err != nil {
			return nil, 0, err
		}
		return &item, item.expiration(now), nil
	})

	if err != nil {
		return nil, err
	}
	return v.(*itemWithExp).val, nil
}

////////////////////////////////////////////////////////////////////////////////

// formatVersionByte indicates what serialization format is used, it is stored
// as a first byte of the serialized data.
//
// Serialized items with different value of the first byte are rejected.
const formatVersionByte = 1

// options is collection of options for GetOrCreate.
type options struct {
	minTTL           time.Duration
	expRandThreshold time.Duration
	allowedStaleness bool
}

type minTTLOpt time.Duration
type expRandThresholdOpt time.Duration
type allowedStalenessOpt struct{}

func (o minTTLOpt) apply(opts *options)           { opts.minTTL = time.Duration(o) }
func (o expRandThresholdOpt) apply(opts *options) { opts.expRandThreshold = time.Duration(o) }
func (o allowedStalenessOpt) apply(opts *options) { opts.allowedStaleness = true }

// itemWithExp is what is actually stored (pointer to it) in the process cache.
//
// It is a user-generated value plus its expiration time (or zero time if it
// doesn't expire).
type itemWithExp struct {
	val interface{}
	exp time.Time

	// Used exclusively by tryReuseStale.
	retries int
	origExp time.Time
}

// isAcceptableTTL returns true if item's TTL is large enough.
func (i *itemWithExp) isAcceptableTTL(now time.Time, minTTL time.Duration) bool {
	if i.exp.IsZero() {
		return true // never expires
	}
	// Note: '>=' must not be used here, since minTTL may be 0, and we don't want
	// to return true on zero expiration.
	return i.exp.Sub(now) > minTTL
}

// randomlyExpired returns true if the item must be considered already expired.
//
// See WithRandomizedExpiration for the rationale. The context is used only to
// grab RNG.
func (i *itemWithExp) randomlyExpired(ctx context.Context, now time.Time, threshold time.Duration) bool {
	if i.exp.IsZero() {
		return false // never expires
	}

	ttl := i.exp.Sub(now)
	if ttl > threshold {
		return false // far from expiration, no need to enable randomization
	}

	// TODO(vadimsh): The choice of distribution here was made arbitrary. Some
	// literature suggests to use exponential distribution instead, but it's not
	// clear how to pick parameters for it. In practice what we do here seems good
	// enough. On each check we randomly expire the item with probability
	// p = (threshold - ttl) / threshold. Closer the item to its true expiration
	// (ttl is smaller), higher the probability.
	rnd := time.Duration(mathrand.Int63n(ctx, int64(threshold)))
	return rnd > ttl
}

// expiration returns expiration time to use when storing this item.
//
// Zero return value means "does not expire" (as understood by both LRU and
// Global caches). Panics if the calculated expiration is negative. Use
// isAcceptableTTL to detect this case beforehand.
func (i *itemWithExp) expiration(now time.Time) time.Duration {
	if i.exp.IsZero() {
		return 0 // never expires
	}
	d := i.exp.Sub(now)
	if d <= 0 {
		panic("item is already expired, isAcceptableTTL should have detected this")
	}
	return d
}

// tryReuseStale chooses either `local` or `global`, modifies its locally stored
// expiration time and returns it so it can be reused in GetOrCreate.
//
// See WithAllowedStaleness for more info.
//
// Returns nil if both items are nil.
func tryReuseStale(ctx context.Context, now time.Time, o *options, local, global *itemWithExp) *itemWithExp {
	// Pick an item we can reuse. Prefer the local one (it has additional state),
	// unless 'global' is fresher.
	var reuse *itemWithExp
	switch {
	case local == nil && global == nil:
		return nil // nothing to reuse at all
	case local != nil && global != nil:
		// Don't use local.exp when comparing against the global.exp, since we
		// adjust it locally when prolonging life of an item in the local LRU
		// (see below). When we do so, we store the original expiration in origExp.
		localExp := local.exp
		if !local.origExp.IsZero() {
			localExp = local.origExp
		}
		if localExp.Equal(global.exp) || localExp.After(global.exp) {
			reuse = local // the local item is as fresh or fresher than the global one
		} else {
			reuse = global // someone updated the global and it is fresher
		}
	case local != nil:
		reuse = local
	case global != nil:
		reuse = global
	}

	// We are about to clobber reuse.exp with a new value, save the original one.
	reuse.retries++
	if reuse.retries == 1 {
		reuse.origExp = reuse.exp
	}

	// We are going to reuse the stale item, but we still want to update it as
	// soon as possible. To do that, we set its expiration time in the local LRU
	// to some near future. This will essentially triggers a retry later.
	//
	// Note that we can't just keep using the original expiration (it is likely in
	// the past already). If we do, we'll just be busy-looping trying to refresh
	// the item.
	exp := time.Duration(math.Pow(1.5, float64(reuse.retries)) * float64(time.Second))
	if exp > 5*time.Minute {
		exp = 5 * time.Minute
	}
	jitter := time.Duration(mathrand.Int63n(ctx, int64(exp)/10) - int64(exp)/20)
	total := o.minTTL + exp + jitter
	reuse.exp = now.Add(total)

	return reuse
}

// maybeFetchItem attempts to fetch the item from the global cache.
//
// If the global cache is not available or the cached item there is broken
// returns nil. Logs errors inside.
func (c *Cache) maybeFetchItem(ctx context.Context, key string) *itemWithExp {
	g := caching.GlobalCache(ctx, c.GlobalNamespace)
	if g == nil {
		return nil
	}

	blob, err := g.Get(ctx, key)
	if err != nil {
		if err != caching.ErrCacheMiss {
			logging.WithError(err).Errorf(ctx, "Failed to read item %q from the global cache", key)
		}
		return nil
	}

	item, err := c.deserializeItem(blob)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to deserialize item %q", key)
		return nil
	}
	return item
}

// maybeStoreItem puts the item in the global cache, if possible.
//
// Uses given `exp` for the expiration duration ofthe item in the global cache.
// It is not the same as item.exp when using WithAllowedStaleness.
//
// Returns an error only if the serialization fails. It generally means the
// serialization code is buggy and should be adjusted.
//
// Global cache errors are logged and ignored.
func (c *Cache) maybeStoreItem(ctx context.Context, key string, item *itemWithExp, exp time.Duration) error {
	g := caching.GlobalCache(ctx, c.GlobalNamespace)
	if g == nil {
		return nil
	}

	blob, err := c.serializeItem(item)
	if err != nil {
		return err
	}

	if err = g.Set(ctx, key, blob, exp); err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to store item %q in the global cache", key)
	}
	return nil
}

// serializeItem packs item and its expiration time into a byte blob.
func (c *Cache) serializeItem(item *itemWithExp) ([]byte, error) {
	blob, err := c.Marshal(item.val)
	if err != nil {
		return nil, err
	}

	var deadline uint64
	if !item.exp.IsZero() {
		deadline = uint64(item.exp.Unix())
	}

	// <version_byte> + <uint64 deadline timestamp> + <blob>
	output := make([]byte, 9+len(blob))
	output[0] = formatVersionByte
	binary.LittleEndian.PutUint64(output[1:], deadline)
	copy(output[9:], blob)
	return output, nil
}

// deserializeItem is reverse of serializeItem.
func (c *Cache) deserializeItem(blob []byte) (item *itemWithExp, err error) {
	if len(blob) < 9 {
		err = fmt.Errorf("the received buffer is too small")
		return
	}
	if blob[0] != formatVersionByte {
		err = fmt.Errorf("bad format version, expecting %d, got %d", formatVersionByte, blob[0])
		return
	}
	item = &itemWithExp{}
	deadline := binary.LittleEndian.Uint64(blob[1:])
	if deadline != 0 {
		item.exp = time.Unix(int64(deadline), 0)
	}
	item.val, err = c.Unmarshal(blob[9:])
	return
}
