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
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/caching"
)

// tokenCache knows how to keep tokens in a cache represented by Cache.
//
// It implements probabilistic early expiration to workaround cache stampede
// problem for hot items, see Fetch.
type tokenCache struct {
	// Kind defines the token kind. Will be used as part of the cache key.
	Kind string

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

// mintTokenResult is the result of a token operation.
type mintTokenResult struct {
	// tok is the retrieved token. It will be nil if no token was retrieved.
	tok *cachedToken
	// label is a string representing the status of the cache operation.
	label string
}

type tokenCacheOp struct {
	CacheKey string
	MinTTL   time.Duration
	Mint     func(context.Context) (mintTokenResult, error)

	// Label will be set by FetchOrMint to describe the context of its result.
	Label string
}

// cachedToken is stored in a tokenCache.
type cachedToken struct {
	Key     string      // cache key, must be unique (no other restrictions)
	Token   interface{} // the actual token, must be Gob-serializable
	Created time.Time   // when it was created, required, UTC
	Expiry  time.Time   // its expiration time, required, UTC
}

type tokenCacheKey string

// Fetch grabs cached token if it hasn't expired yet.
//
// Returns (nil, nil) if no such token, it has expired already or its lifetime
// is shorter than 'minTTL'.
//
// If the cached token is close to expiration cutoff (defined by minTTL), this
// function will randomly return a cache miss. That way if multiple concurrent
// processes all constantly use the same token, only the most unlucky one will
// refresh it.
func (tc *tokenCache) FetchOrMint(ctx context.Context, op *tokenCacheOp) (*cachedToken, error) {
	pc := caching.ProcessCache(ctx)
	cacheKey := tc.itemKey(op.CacheKey)

	// Try and get the token while it's not under lock.
	tokIface, ok := pc.Get(ctx, cacheKey)
	if ok {
		tok := tokIface.(*cachedToken)
		if tok.Key != op.CacheKey {
			op.Label = "ERROR_CACHE"
			return nil, fmt.Errorf("SHA1 collision in the token cache: %q vs %q", op.CacheKey, tok.Key)
		}

		// Shift time by minTTL to simplify the math below.
		if !tc.shouldRefreshToken(ctx, tok, op.MinTTL) {
			op.Label = "SUCCESS_CACHE_HIT"
			return tok, nil // far from requested minTTL
		}
	}

	// Either the token has expired, or we were randomly chosen to refresh it.
	tokIface, err := caching.ProcessCache(ctx).Create(ctx, cacheKey, func() (interface{}, time.Duration, error) {
		logging.Debugf(ctx, "Minting the new token")
		mintResult, err := op.Mint(ctx)
		op.Label = mintResult.label
		if err != nil {
			return nil, 0, err
		}
		tok := mintResult.tok

		tok.Key = op.CacheKey
		op.Label = "SUCCESS_CACHE_MISS"

		switch {
		case tok.Created.IsZero() || tok.Created.Location() != time.UTC:
			panic(fmt.Errorf("tok.Created is not a valid UTC time - %v", tok.Created))
		case tok.Expiry.IsZero() || tok.Expiry.Location() != time.UTC:
			panic(fmt.Errorf("tok.Expiry is not a valid UTC time - %v", tok.Expiry))
		}
		ttl := tok.Expiry.Sub(tok.Created)
		if ttl < tc.MinAcceptedLifetime {
			return nil, 0, fmt.Errorf("refusing to store a token that expires in %s", ttl)
		}

		return tok, ttl, nil
	})
	if err != nil {
		if op.Label == "" {
			op.Label = "ERROR_UNSPECIFIED"
		}

		logging.WithError(err).Errorf(ctx, "Failed to mint new token")
		return nil, err
	}
	return tokIface.(*cachedToken), err
}

func (tc *tokenCache) shouldRefreshToken(ctx context.Context, tok *cachedToken, minTTL time.Duration) bool {
	now := clock.Now(ctx).Add(minTTL).UTC()

	// If the token is expired, then refresh.
	if now.After(tok.Expiry) {
		return true
	}

	// Don't use expiration randomization if the item is far from expiration.
	exp := tok.Expiry.Sub(tok.Created) * time.Duration(tc.ExpRandPercent) / 100
	if now.Add(exp).Before(tok.Expiry) {
		return false
	}

	// The expiration is close enough. Do the randomization.
	// TODO(vadimsh): The distribution was picked randomly. Maybe use exponential
	// one, as some literature suggests?
	rnd := time.Duration(mathrand.Int63n(ctx, int64(exp)))
	if !now.Add(rnd).After(tok.Expiry) {
		return false
	}

	return true
}

// itemKey derives the short key to use in the underlying cache.
func (tc *tokenCache) itemKey(key string) tokenCacheKey {
	digest := sha1.Sum([]byte(key))
	asStr := base64.RawURLEncoding.EncodeToString(digest[:])
	return tokenCacheKey(fmt.Sprintf("%s/%s", tc.Kind, asStr))
}
