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
	"math/rand"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTokenCache(t *testing.T) {
	t.Parallel()

	Convey("with in-process cache", t, func() {
		ctx := context.Background()
		ctx, clk := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))

		cache := lru.New(0)
		ctx = caching.WithProcessCache(ctx, cache)

		tc := tokenCache{
			Kind:           "testing",
			ExpRandPercent: 10,
		}

		mintCalls := 0
		example := &cachedToken{
			Key:     "some\nkey",
			Token:   "blah",
			Created: clock.Now(ctx).UTC(),
			Expiry:  clock.Now(ctx).Add(100 * time.Minute).UTC(),
		}

		op := tokenCacheOp{
			CacheKey: "some\nkey",
			MinTTL:   21 * time.Minute,
			Mint: func(context.Context) (mintTokenResult, error) {
				mintCalls++
				return mintTokenResult{example, "MINTED"}, nil
			},
		}

		Convey("check basic usage", func() {
			tok, err := tc.FetchOrMint(ctx, &op)
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, example)
			So(mintCalls, ShouldEqual, 1)

			tok, err = tc.FetchOrMint(ctx, &op)
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, example)
			So(mintCalls, ShouldEqual, 1)

			// minTTL works. After 80 min the tokens TTL is 20 min, but we request it
			// to be at least 21 min.
			clk.Add(80 * time.Minute)
			tok, err = tc.FetchOrMint(ctx, &op)
			So(err, ShouldBeNil)
			So(tok, ShouldNotBeNil)
			So(mintCalls, ShouldEqual, 2)

			So(cache.Len(), ShouldEqual, 1)
		})

		Convey("check expiration randomization", func() {
			op.MinTTL = 0

			tok, err := tc.FetchOrMint(ctx, &op)
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, example)
			So(mintCalls, ShouldEqual, 1)

			// 89% of token's life has passed. The token is still alive
			// (no randomization).
			clk.Add(89 * time.Minute)
			for i := 0; i < 50; i++ {
				tok, err := tc.FetchOrMint(ctx, &op)
				So(err, ShouldBeNil)
				So(tok, ShouldResemble, example)
				So(mintCalls, ShouldEqual, 1)
			}

			// 91% of token's life has passed. 'Fetch' pretends the token is not
			// there with ~=10% chance.
			clk.Add(2 * time.Minute)
			mintCalls = 0
			for i := 0; i < 100; i++ {
				_, err := tc.FetchOrMint(ctx, &op)
				So(err, ShouldBeNil)
			}
			So(mintCalls, ShouldEqual, 10)

			// Token has expired. 0% chance of seeing it.
			clk.Add(9 * time.Minute)
			mintCalls = 0
			for i := 0; i < 50; i++ {
				_, err := tc.FetchOrMint(ctx, &op)
				So(err, ShouldBeNil)
			}
			So(mintCalls, ShouldEqual, 50)
		})
	})
}
