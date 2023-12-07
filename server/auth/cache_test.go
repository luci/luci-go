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

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
	"go.chromium.org/luci/server/caching/layered"

	. "github.com/smartystreets/goconvey/convey"
)

var testCache = newTokenCache(tokenCacheConfig{
	Kind:                 "testing",
	ProcessCacheCapacity: 9,
})

func TestTokenCache(t *testing.T) {
	t.Parallel()

	Convey("with in-process cache", t, func() {
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

		Convey("Basic usage", func() {
			// Generate initial token.
			tok1 := makeTestToken(ctx, "token-1")
			tok, err, label := call(tok1, nil, "")
			So(err, ShouldBeNil)
			So(label, ShouldEqual, "SUCCESS_CACHE_MISS")
			So(tok, ShouldEqual, tok1)

			// Some time later still cache hit.
			tc.Add(49 * time.Minute)
			tok, err, label = call(nil, errors.New("must not be called"), "")
			So(err, ShouldBeNil)
			So(label, ShouldEqual, "SUCCESS_CACHE_HIT")
			So(tok, ShouldEqual, tok1)

			// Lifetime of the existing token is not good enough => refreshed.
			tc.Add(2 * time.Minute)
			tok2 := makeTestToken(ctx, "token-1")
			tok, err, label = call(tok2, nil, "")
			So(err, ShouldBeNil)
			So(label, ShouldEqual, "SUCCESS_CACHE_MISS")
			So(tok, ShouldEqual, tok2)
		})

		Convey("Marshalling works", func() {
			// Generate initial token.
			tok1 := makeTestToken(ctx, "token-1")
			tok, err, label := call(tok1, nil, "")
			So(err, ShouldBeNil)
			So(label, ShouldEqual, "SUCCESS_CACHE_MISS")
			So(tok, ShouldEqual, tok1)

			// Kick it out of local cache by wiping it. It still in global cache.
			ctx = caching.WithEmptyProcessCache(ctx)

			// Cache hit through the global cache.
			tok, err, label = call(nil, errors.New("must not be called"), "")
			So(err, ShouldBeNil)
			So(label, ShouldEqual, "SUCCESS_CACHE_HIT")
			So(tok.Created.Equal(tok1.Created), ShouldBeTrue)
			So(tok.Expiry.Equal(tok1.Expiry), ShouldBeTrue)
			So(tok.OAuth2Token, ShouldEqual, tok1.OAuth2Token)
		})

		Convey("Mint error", func() {
			err := errors.New("some error")
			tok, err, label := call(nil, err, "SOME_LABEL")
			So(tok, ShouldBeNil)
			So(err, ShouldEqual, err)
			So(label, ShouldEqual, "SOME_LABEL")

			tok, err, label = call(nil, err, "")
			So(tok, ShouldBeNil)
			So(err, ShouldEqual, err)
			So(label, ShouldEqual, "ERROR_UNSPECIFIED")
		})

		Convey("Small TTL", func() {
			tok1 := makeTestToken(ctx, "token")
			tok1.Expiry = tok1.Created.Add(time.Second)

			tok, err, label := call(tok1, nil, "")
			So(tok, ShouldBeNil)
			So(err, ShouldEqual, layered.ErrCantSatisfyMinTTL)
			So(label, ShouldEqual, "ERROR_INSUFFICIENT_MINTED_TTL")
		})
	})
}
