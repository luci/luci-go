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
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/jsontime"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/delegation"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"
)

// globalCacheNamespace is global cache namespace to use for storing tokens.
const globalCacheNamespace = "__luciauth__"

// tokenCacheConfig contains configuration of a token cache for a single token
// kind.
type tokenCacheConfig struct {
	// Kind defines the token kind. Will be used as part of the global cache key.
	Kind string

	// Version defines format of the data. Will be used as part of the global
	// cache key.
	//
	// If you change a type behind interface{} in Token field, you MUST bump the
	// version. It will also "invalidate" all existing cached entries (they will
	// just become inaccessible and eventually will be evicted from the cache).
	Version int

	// ProcessLRUCache is a handle to a process LRU cache that holds the tokens.
	ProcessLRUCache caching.LRUHandle

	// ExpiryRandomizationThreshold defines a threshold for item expiration after
	// which the randomized early expiration kick in.
	//
	// See layered.WithRandomizedExpiration for more details.
	ExpiryRandomizationThreshold time.Duration
}

// tokenCache knows how to store tokens of some particular kind.
//
// Must be initialized during init-time via newTokenCache.
type tokenCache struct {
	cfg tokenCacheConfig
	lc  layered.Cache
}

// newTokenCache configures tokenCache based on given parameters.
func newTokenCache(cfg tokenCacheConfig) *tokenCache {
	return &tokenCache{
		cfg: cfg,
		lc: layered.Cache{
			ProcessLRUCache: cfg.ProcessLRUCache,
			GlobalNamespace: globalCacheNamespace,
			Marshal:         json.Marshal, // marshals *cachedToken
			Unmarshal: func(blob []byte) (interface{}, error) {
				out := &cachedToken{}
				err := json.Unmarshal(blob, out)
				return out, err
			},
		},
	}
}

// cachedToken is stored in the token cache.
type cachedToken struct {
	// Key is cache key, must be unique (no other restrictions).
	Key string `json:"key,omitempty"`
	// Created is when the token was created, required, UTC.
	Created jsontime.Time `json:"created,omitempty"`
	// Expire is when the token expires, required, UTC.
	Expiry jsontime.Time `json:"expiry,omitempty"`

	// OAuth2Token is set when caching an OAuth2 tokens, otherwise nil.
	OAuth2Token *cachedOAuth2Token `json:"oauth2_token,omitempty"`
	// DelegationToken is set when caching a delegation token, otherwise nil.
	DelegationToken *delegation.Token `json:"delegation_token,omitempty"`
}

type fetchOrMintTokenOp struct {
	CacheKey    string
	MinTTL      time.Duration
	Mint        func(context.Context) (tok *cachedToken, err error, label string)
	MintTimeout time.Duration
}

// fetchOrMintToken implements high level logic of using a token cache.
//
// It's basis or MintAccessTokenForServiceAccount and MintDelegationToken
// implementations.
//
// Returns a token, an error and a label to use for monitoring metric (its value
// depends on how exactly the operation was performed or how it failed).
func (tc *tokenCache) fetchOrMintToken(ctx context.Context, op *fetchOrMintTokenOp) (tok *cachedToken, err error, label string) {
	defer func() {
		if err != nil {
			logging.WithError(err).Errorf(ctx, "Failed to get the token")
		}
	}()

	// Derive a short unique cache key that also depends on Kind and Version.
	// op.CacheKey is allowed to be of any length, but global cache keys must be
	// short-ish.
	digest := sha1.Sum([]byte(op.CacheKey))
	cacheKey := fmt.Sprintf("%s/%d/%s",
		tc.cfg.Kind, tc.cfg.Version, base64.RawURLEncoding.EncodeToString(digest[:]))

	label = "SUCCESS_CACHE_HIT" // will be replaced on cache miss or on error

	// Pull our token from the cache or create a new one (construct options first
	// for better readability).
	opts := []layered.Option{
		layered.WithMinTTL(op.MinTTL),
		layered.WithRandomizedExpiration(tc.cfg.ExpiryRandomizationThreshold),
	}
	fetched, err := tc.lc.GetOrCreate(ctx, cacheKey, func() (val interface{}, ttl time.Duration, err error) {
		logging.Debugf(ctx, "Minting the new token")

		// Minting a new token involves RPCs to remote services that should be fast.
		// Abort the attempt if it gets stuck for longer than N sec, it's unlikely
		// it'll succeed. Note that we setup the new context only on slow code path
		// (on cache miss), since it involves some overhead we don't want to pay on
		// the fast path. We assume memcache RPCs don't get stuck for a long time
		// (unlike URL Fetch calls to GAE).
		ctx, cancel := clock.WithTimeout(ctx, op.MintTimeout)
		defer cancel()

		// Note: we set 'label' from the outer scope here.
		var tok *cachedToken
		if tok, err, label = op.Mint(ctx); err != nil {
			if label == "" {
				label = "ERROR_UNSPECIFIED"
			}
			return nil, 0, err
		}

		tok.Key = op.CacheKey // the original key before hashing

		label = "SUCCESS_CACHE_MISS"
		return tok, clock.Until(ctx, tok.Expiry.Time), nil
	}, opts...)

	switch tok, _ = fetched.(*cachedToken); {
	case errors.Unwrap(err) == context.DeadlineExceeded:
		return nil, err, "ERROR_DEADLINE"
	case err == layered.ErrCantSatisfyMinTTL:
		// This happens if op.Mint failed to produce a token that lives longer
		// than MinTTL.
		return nil, err, "ERROR_INSUFFICIENT_MINTED_TTL"
	case err != nil:
		return nil, err, label
	case tok.Key != op.CacheKey:
		// A paranoid check we've got the token we wanted. This is very-very-very
		// unlikely to happen in practice, SHA1 collisions are rare. So it's fine to
		// handle it sloppily and just return an error (still better than
		// accidentally using wrong token).
		err = fmt.Errorf("SHA1 collision in the token cache: %q vs %q", tok.Key, op.CacheKey)
		return nil, err, "ERROR_HASH_COLLISION"
	default:
		return tok, nil, label
	}
}
