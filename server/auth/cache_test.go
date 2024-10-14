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
	"errors"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
	"go.chromium.org/luci/server/caching/layered"
)

var testCache = newTokenCache(tokenCacheConfig{
	Kind:                 "testing",
	ProcessCacheCapacity: 9,
})

func TestTokenCache(t *testing.T) {
	t.Parallel()

	ftt.Run("with in-process cache", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			globalCacheNamespace: cachingtest.NewBlobCache(),
		})

		makeTestToken := func(ctx context.Context, val string) *cachedToken {
			return &cachedToken{
				Created:     clock.Now(ctx),
				Expiry:      clock.Now(ctx).Add(time.Hour),
				OAuth2Token: val,
			}
		}

		call := func(mocked *cachedToken, err error, label string) (*cachedToken, error, string) {
			return testCache.fetchOrMintToken(ctx, &fetchOrMintTokenOp{
				CacheKey: "key",
				MinTTL:   10 * time.Minute,
				Mint: func(ctx context.Context) (*cachedToken, error, string) {
					return mocked, err, label
				},
			})
		}

		t.Run("Basic usage", func(t *ftt.Test) {
			// Generate initial token.
			tok1 := makeTestToken(ctx, "token-1")
			tok, err, label := call(tok1, nil, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, label, should.Equal("SUCCESS_CACHE_MISS"))
			assert.Loosely(t, tok, should.Equal(tok1))

			// Some time later still cache hit.
			tc.Add(49 * time.Minute)
			tok, err, label = call(nil, errors.New("must not be called"), "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, label, should.Equal("SUCCESS_CACHE_HIT"))
			assert.Loosely(t, tok, should.Equal(tok1))

			// Lifetime of the existing token is not good enough => refreshed.
			tc.Add(2 * time.Minute)
			tok2 := makeTestToken(ctx, "token-1")
			tok, err, label = call(tok2, nil, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, label, should.Equal("SUCCESS_CACHE_MISS"))
			assert.Loosely(t, tok, should.Equal(tok2))
		})

		t.Run("Marshalling works", func(t *ftt.Test) {
			// Generate initial token.
			tok1 := makeTestToken(ctx, "token-1")
			tok, err, label := call(tok1, nil, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, label, should.Equal("SUCCESS_CACHE_MISS"))
			assert.Loosely(t, tok, should.Equal(tok1))

			// Kick it out of local cache by wiping it. It still in global cache.
			ctx = caching.WithEmptyProcessCache(ctx)

			// Cache hit through the global cache.
			tok, err, label = call(nil, errors.New("must not be called"), "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, label, should.Equal("SUCCESS_CACHE_HIT"))
			assert.Loosely(t, tok.Created.Equal(tok1.Created), should.BeTrue)
			assert.Loosely(t, tok.Expiry.Equal(tok1.Expiry), should.BeTrue)
			assert.Loosely(t, tok.OAuth2Token, should.Equal(tok1.OAuth2Token))
		})

		t.Run("Mint error", func(t *ftt.Test) {
			err := errors.New("some error")
			tok, err, label := call(nil, err, "SOME_LABEL")
			assert.Loosely(t, tok, should.BeNil)
			assert.Loosely(t, err, should.Equal(err))
			assert.Loosely(t, label, should.Equal("SOME_LABEL"))

			tok, err, label = call(nil, err, "")
			assert.Loosely(t, tok, should.BeNil)
			assert.Loosely(t, err, should.Equal(err))
			assert.Loosely(t, label, should.Equal("ERROR_UNSPECIFIED"))
		})

		t.Run("Small TTL", func(t *ftt.Test) {
			tok1 := makeTestToken(ctx, "token")
			tok1.Expiry = tok1.Created.Add(time.Second)

			tok, err, label := call(tok1, nil, "")
			assert.Loosely(t, tok, should.BeNil)
			assert.Loosely(t, err, should.Equal(layered.ErrCantSatisfyMinTTL))
			assert.Loosely(t, label, should.Equal("ERROR_INSUFFICIENT_MINTED_TTL"))
		})
	})
}
