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

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/data/rand/mathrand"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTokenCache(t *testing.T) {
	t.Parallel()

	Convey("with in-process cache", t, func() {
		// Create a cache large enough that the LRU doesn't cycle.
		cache := MemoryCache(1024)

		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))

		tc := tokenCache{
			Kind:           "testing",
			Version:        1,
			ExpRandPercent: 10,
		}

		Convey("check basic usage", func() {
			itm, err := tc.Fetch(ctx, cache, "some\nkey", 0)
			So(err, ShouldBeNil)
			So(itm, ShouldBeNil)

			tok := cachedToken{
				Key:     "some\nkey",
				Token:   "blah",
				Created: clock.Now(ctx).UTC(),
				Expiry:  clock.Now(ctx).Add(100 * time.Minute).UTC(),
			}
			So(tc.Store(ctx, cache, &tok), ShouldBeNil)

			itm, err = tc.Fetch(ctx, cache, "some\nkey", 0)
			So(err, ShouldBeNil)
			So(itm, ShouldResemble, &tok)

			// minTTL works. After 80 min the tokens TTL is 20 min, but we request it
			// to be at least 21 min.
			clock.Get(ctx).(testclock.TestClock).Add(80 * time.Minute)
			itm, err = tc.Fetch(ctx, cache, "some\nkey", 21*time.Minute)
			So(err, ShouldBeNil)
			So(itm, ShouldBeNil)

			pgc := cache.(*memoryCache)
			So(pgc.cache.Len(), ShouldEqual, 1)
		})

		Convey("check expiration randomization", func() {
			tok := cachedToken{
				Key:     "some\nkey",
				Token:   "blah",
				Created: clock.Now(ctx).UTC(),
				Expiry:  clock.Now(ctx).Add(100 * time.Minute).UTC(),
			}
			So(tc.Store(ctx, cache, &tok), ShouldBeNil)

			// 89% of token's life has passed. The token is still alive
			// (no randomization).
			clock.Get(ctx).(testclock.TestClock).Add(89 * time.Minute)
			for i := 0; i < 50; i++ {
				itm, _ := tc.Fetch(ctx, cache, "some\nkey", 0)
				So(itm, ShouldNotBeNil)
			}

			// 91% of token's life has passed. 'Fetch' pretends the token is not
			// there with ~=10% chance.
			clock.Get(ctx).(testclock.TestClock).Add(2 * time.Minute)
			missing := 0
			for i := 0; i < 100; i++ {
				if itm, _ := tc.Fetch(ctx, cache, "some\nkey", 0); itm == nil {
					missing++
				}
			}
			So(missing, ShouldEqual, 10)

			// Token has expired. 0% chance of seeing it.
			clock.Get(ctx).(testclock.TestClock).Add(9 * time.Minute)
			for i := 0; i < 50; i++ {
				itm, _ := tc.Fetch(ctx, cache, "some\nkey", 0)
				So(itm, ShouldBeNil)
			}
		})
	})
}
