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

package auth

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/caching/lru"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/sync/mutexpool"
)

// Cache implements a strongly consistent cache.
type Cache interface {
	// Get returns a cached item or (nil, nil) if it's not in the cache.
	//
	// Any returned error is transient error.
	Get(c context.Context, key string) ([]byte, error)

	// Set unconditionally overwrites an item in the cache.
	//
	// If 'exp' is zero, the item will have no expiration time.
	//
	// Any returned error is transient error.
	Set(c context.Context, key string, value []byte, exp time.Duration) error

	// WithLocalMutex calls 'f' under a process-local mutex assigned to the 'key'.
	//
	// It is used to ensure only one goroutine is updating the item matching the
	// given key.
	WithLocalMutex(c context.Context, key string, f func())
}

// tokenCache knows how to keep tokens in a cache represented by Cache.
//
// It implements probabilistic early expiration to workaround cache stampede
// problem for hot items, see Fetch.
type tokenCache struct {
	// Kind defines the token kind. Will be used as part of the cache key.
	Kind string

	// Version defines format of the data. Will be used as part of the cache key.
	//
	// If you change a type behind interface{} in Token field, you MUST bump the
	// version. It will also "invalidate" all existing cached entries (they will
	// just become inaccessible and eventually will be evicted from the cache).
	Version int

	// ExpRandPercent defines when expiration randomizations kicks in.
	//
	// For example, value of '10' means when token is within 10% of its
	// expiration it may randomly (on each access) be considered already expired.
	ExpRandPercent int

	// MinAcceptedLifetime defines minimal lifetime of a token.
	//
	// If a token lives less than given time, Store(...) will refuse to save it.
	MinAcceptedLifetime time.Duration
}

// cachedToken is stored in a tokenCache.
type cachedToken struct {
	Key     string      // cache key, must be unique (no other restrictions)
	Token   interface{} // the actual token, must be Gob-serializable
	Created time.Time   // when it was created, required, UTC
	Expiry  time.Time   // its expiration time, required, UTC
}

// Store unconditionally puts the token in the cache.
func (tc *tokenCache) Store(c context.Context, cache Cache, tok *cachedToken) error {
	switch {
	case tok.Created.IsZero() || tok.Created.Location() != time.UTC:
		panic(fmt.Errorf("tok.Created is not a valid UTC time - %v", tok.Created))
	case tok.Expiry.IsZero() || tok.Expiry.Location() != time.UTC:
		panic(fmt.Errorf("tok.Expiry is not a valid UTC time - %v", tok.Expiry))
	}
	ttl := tok.Expiry.Sub(tok.Created)
	if ttl < tc.MinAcceptedLifetime {
		return fmt.Errorf("refusing to store a token that expires in %s", ttl)
	}
	blob, err := tc.marshal(tok)
	if err != nil {
		return err
	}
	return cache.Set(c, tc.itemKey(tok.Key), blob, ttl)
}

// Fetch grabs cached token if it hasn't expired yet.
//
// Returns (nil, nil) if no such token, it has expired already or its lifetime
// is shorter than 'minTTL'.
//
// If the cached token is close to expiration cutoff (defined by minTTL), this
// function will randomly return a cache miss. That way if multiple concurrent
// processes all constantly use the same token, only the most unlucky one will
// refresh it.
func (tc *tokenCache) Fetch(c context.Context, cache Cache, key string, minTTL time.Duration) (*cachedToken, error) {
	blob, err := cache.Get(c, tc.itemKey(key))
	switch {
	case err != nil:
		return nil, err
	case blob == nil:
		return nil, nil
	}

	tok, err := tc.unmarshal(blob)
	if err != nil {
		return nil, err
	}
	if tok.Key != key {
		return nil, fmt.Errorf("SHA1 collision in the token cache: %q vs %q", key, tok.Key)
	}

	// Shift time by minTTL to simplify the math below.
	now := clock.Now(c).Add(minTTL).UTC()

	// Don't use expiration randomization if the item is far from expiration.
	exp := tok.Expiry.Sub(tok.Created) * time.Duration(tc.ExpRandPercent) / 100
	switch {
	case now.After(tok.Expiry):
		return nil, nil // already passed requested minTTL
	case now.Add(exp).Before(tok.Expiry):
		return tok, nil // far from requested minTTL
	}

	// The expiration is close enough. Do the randomization.
	// TODO(vadimsh): The distribution was picked randomly. Maybe use exponential
	// one, as some literature suggests?
	rnd := time.Duration(mathrand.Int63n(c, int64(exp)))
	if now.Add(rnd).After(tok.Expiry) {
		return nil, nil
	}
	return tok, nil
}

// WithLocalMutex calls Cache.WithLocalMutex with correct cache key.
func (tc *tokenCache) WithLocalMutex(c context.Context, cache Cache, key string, f func()) {
	cache.WithLocalMutex(c, tc.itemKey(key), f)
}

// itemKey derives the short key to use in the underlying cache.
func (tc *tokenCache) itemKey(key string) string {
	digest := sha1.Sum([]byte(key))
	asStr := base64.RawURLEncoding.EncodeToString(digest[:])
	return fmt.Sprintf("%s/%d/%s", tc.Kind, tc.Version, asStr)
}

// marshal converts cachedToken struct to byte blob.
func (tc *tokenCache) marshal(t *cachedToken) ([]byte, error) {
	buf := bytes.Buffer{}
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(t); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unmarshal is reverse of marshal.
func (tc *tokenCache) unmarshal(blob []byte) (*cachedToken, error) {
	dec := gob.NewDecoder(bytes.NewReader(blob))
	out := &cachedToken{}
	if err := dec.Decode(out); err != nil {
		return nil, err
	}
	return out, nil
}

////////////////////////////////////////////////////////////////////////////////

type fetchOrMintTokenOp struct {
	Config   *Config
	Cache    *tokenCache
	CacheKey string
	MinTTL   time.Duration
	Mint     func(context.Context) (tok *cachedToken, err error, label string)
}

// fetchOrMintToken implements high level logic of using token cache.
//
// It's basis or MintAccessTokenForServiceAccount and MintDelegationToken
// implementations.
func fetchOrMintToken(ctx context.Context, op *fetchOrMintTokenOp) (*cachedToken, error, string) {
	// Attempt to grab a cached token before entering the critical section.
	switch cached, err := op.Cache.Fetch(ctx, op.Config.Cache, op.CacheKey, op.MinTTL); {
	case err != nil:
		logging.WithError(err).Errorf(ctx, "Failed to fetch the token from cache")
		return nil, err, "ERROR_CACHE"
	case cached != nil:
		return cached, nil, "SUCCESS_CACHE_HIT"
	}

	logging.Debugf(ctx, "The requested token is not in the cache")

	var res struct {
		tok   *cachedToken
		err   error
		label string
	}
	op.Cache.WithLocalMutex(ctx, op.Config.Cache, op.CacheKey, func() {
		// Recheck the cache now that we have the lock, maybe someone updated the
		// cache while we were waiting.
		switch cached, err := op.Cache.Fetch(ctx, op.Config.Cache, op.CacheKey, op.MinTTL); {
		case err != nil:
			logging.WithError(err).Errorf(ctx, "Failed to fetch the token from cache (under the lock)")
			res.err = err
			res.label = "ERROR_CACHE"
			return
		case cached != nil:
			logging.Debugf(ctx, "Someone put the token in the cache already")
			res.tok = cached
			res.label = "SUCCESS_CACHE_HIT"
			return
		}

		// Minting a new token involves RPCs to remote services that should be fast.
		// Abort the attempt if it gets stuck for longer than 10 sec, it's unlikely
		// it'll succeed. Note that we setup the new context only on slow code path
		// (on cache miss), since it involves some overhead we don't want to pay on
		// the fast path. We assume memcache RPCs don't get stuck for a long time
		// (unlike URL Fetch calls to GAE).
		logging.Debugf(ctx, "Minting the new token")
		ctx, cancel := clock.WithTimeout(ctx, op.Config.adjustedTimeout(10*time.Second))
		defer func() {
			if res.err != nil {
				logging.WithError(res.err).Errorf(ctx, "Failed to mint new token")
			}
			cancel()
		}()
		if res.tok, res.err, res.label = op.Mint(ctx); res.err != nil {
			res.tok = nil
			if res.label == "" {
				res.label = "ERROR_UNSPECIFIED"
			}
			return
		}
		res.tok.Key = op.CacheKey

		// Cache the token. Ignore errors, it's not big deal, we have the token.
		if err := op.Cache.Store(ctx, op.Config.Cache, res.tok); err != nil {
			logging.WithError(err).Warningf(ctx, "Failed to store the token in the cache")
		}

		res.label = "SUCCESS_CACHE_MISS"
	})

	return res.tok, res.err, res.label
}

////////////////////////////////////////////////////////////////////////////////

type memoryCache struct {
	cache *lru.Cache
	locks mutexpool.P
}

// MemoryCache creates a new in-memory Cache instance that is built on
// top of an LRU cache of the specified size.
func MemoryCache(size int) Cache {
	return &memoryCache{
		cache: lru.New(size),
	}
}

func (mc *memoryCache) Get(c context.Context, key string) ([]byte, error) {
	var item *memoryCacheItem
	now := clock.Now(c)
	_ = mc.cache.Mutate(key, func(cur interface{}) interface{} {
		if cur == nil {
			return nil
		}

		item = cur.(*memoryCacheItem)
		if now.After(item.exp) {
			// Cache item is too old, so expire it.
			item = nil
		}
		return item
	})
	if item == nil {
		// Cache miss (or expired).
		return nil, nil
	}
	return item.value, nil
}

func (mc *memoryCache) Set(c context.Context, key string, value []byte, exp time.Duration) error {
	mc.cache.Put(key, &memoryCacheItem{
		value: value,
		exp:   clock.Now(c).Add(exp),
	})
	return nil
}

func (mc *memoryCache) WithLocalMutex(c context.Context, key string, f func()) {
	mc.locks.WithMutex(key, f)
}

type memoryCacheItem struct {
	value []byte
	exp   time.Time
}
