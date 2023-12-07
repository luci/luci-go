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

package layered

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"

	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var testingCache = RegisterCache(Parameters[[]byte]{
	GlobalNamespace: "namespace",
	Marshal: func(item []byte) ([]byte, error) {
		return item, nil
	},
	Unmarshal: func(blob []byte) ([]byte, error) {
		return blob, nil
	},
})

var testingCacheWithFallback = RegisterCache(Parameters[[]byte]{
	GlobalNamespace: "namespace",
	Marshal: func(item []byte) ([]byte, error) {
		return item, nil
	},
	Unmarshal: func(blob []byte) ([]byte, error) {
		return blob, nil
	},
	AllowNoProcessCacheFallback: true,
})

func TestCache(t *testing.T) {
	t.Parallel()

	Convey("Without process cache", t, func() {
		ctx := context.Background()

		_, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
			panic("should not be called")
		})
		So(err, ShouldEqual, caching.ErrNoProcessCache)

		calls := 0
		_, err = testingCacheWithFallback.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
			calls += 1
			return nil, 0, nil
		})
		So(err, ShouldBeNil)
		So(calls, ShouldEqual, 1)
	})

	Convey("With fake time", t, func() {
		ctx := context.Background()
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))
		ctx, tc := testclock.UseTime(ctx, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC))
		ctx = caching.WithEmptyProcessCache(ctx)

		calls := 0
		value := []byte("value")
		anotherValue := []byte("anotherValue")

		getter := func() ([]byte, time.Duration, error) {
			calls++
			return value, time.Hour, nil
		}

		Convey("Without global cache", func() {
			item, err := testingCache.GetOrCreate(ctx, "item", getter)
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)
			So(calls, ShouldEqual, 1)

			tc.Add(59 * time.Minute)

			item, err = testingCache.GetOrCreate(ctx, "item", getter)
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)
			So(calls, ShouldEqual, 1) // no new calls

			tc.Add(2 * time.Minute) // cached item expires

			item, err = testingCache.GetOrCreate(ctx, "item", getter)
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)
			So(calls, ShouldEqual, 2) // new call!
		})

		Convey("With global cache", func() {
			global := cachingtest.NewBlobCache()
			ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
				"namespace": global,
			})

			Convey("Getting from the global cache", func() {
				// The global cache is empty.
				So(global.LRU.Len(), ShouldEqual, 0)

				// Create an item.
				item, err := testingCache.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 1)

				// It is in the global cache now.
				So(global.LRU.Len(), ShouldEqual, 1)

				// Clear the local cache.
				ctx = caching.WithEmptyProcessCache(ctx)

				// Grab the item again. Will be fetched from the global cache.
				item, err = testingCache.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 1) // no new calls
			})

			Convey("Broken global cache is ignored", func() {
				global.Err = errors.New("broken")

				// Create an item.
				item, err := testingCache.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 1)

				// Clear the local cache.
				ctx = caching.WithEmptyProcessCache(ctx)

				// Grab the item again. Will be recreated again, since the global cache
				// is broken.
				item, err = testingCache.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 2) // new call!
			})
		})

		Convey("Never expiring item", func() {
			item, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return value, 0, nil
			})
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)

			tc.Add(100 * time.Hour)

			item, err = testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return nil, 0, errors.New("must not be called")
			})
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)
		})

		Convey("WithMinTTL works", func() {
			item, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return value, time.Hour, nil
			})
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)

			tc.Add(50 * time.Minute)

			// 9 min minTTL is still ok.
			item, err = testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return nil, 0, errors.New("must not be called")
			}, WithMinTTL(9*time.Minute))
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)

			// But 10 min is not and the item is refreshed.
			item, err = testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return anotherValue, time.Hour, nil
			}, WithMinTTL(10*time.Minute))
			So(err, ShouldBeNil)
			So(item, ShouldResemble, anotherValue)
		})

		Convey("ErrCantSatisfyMinTTL", func() {
			_, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return value, time.Minute, nil
			}, WithMinTTL(2*time.Minute))
			So(err, ShouldEqual, ErrCantSatisfyMinTTL)
		})

		oneRandomizedTrial := func(now, threshold time.Duration) (cacheHit bool) {
			// Reset state (except RNG).
			ctx := caching.WithEmptyProcessCache(ctx)
			ctx, tc := testclock.UseTime(ctx, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC))

			// Put the item in the cache.
			item, err := testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return value, time.Hour, nil
			})
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)

			tc.Add(now)

			// Grab it (or trigger a refresh if randomly expired).
			item, err = testingCache.GetOrCreate(ctx, "item", func() ([]byte, time.Duration, error) {
				return anotherValue, time.Hour, nil
			}, WithRandomizedExpiration(threshold))
			So(err, ShouldBeNil)
			return bytes.Equal(item, value)
		}

		testCases := []struct {
			now, threshold  time.Duration
			expectedHitRate int
		}{
			{50 * time.Minute, 10 * time.Minute, 100}, // before threshold, no random expiration
			{51 * time.Minute, 10 * time.Minute, 90},  // slightly above => some expiration
			{59 * time.Minute, 10 * time.Minute, 11},  // almost at the threshold => heavy expiration
			{61 * time.Minute, 10 * time.Minute, 0},   // outside item expiration => always expired
		}

		for i := 0; i < len(testCases); i++ {
			now := testCases[i].now
			threshold := testCases[i].threshold
			expectedHitRate := testCases[i].expectedHitRate

			Convey(fmt.Sprintf("WithRandomizedExpiration (now = %s)", now), func() {
				cacheHits := 0
				for i := 0; i < 100; i++ {
					if oneRandomizedTrial(now, threshold) {
						cacheHits++
					}
				}
				So(cacheHits, ShouldEqual, expectedHitRate)
			})
		}
	})
}

func TestSerialization(t *testing.T) {
	t.Parallel()

	Convey("With cache", t, func() {
		now := time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC)

		c := Cache[[]byte]{
			params: Parameters[[]byte]{
				Marshal: func(item []byte) ([]byte, error) {
					return item, nil
				},
				Unmarshal: func(blob []byte) ([]byte, error) {
					return blob, nil
				},
			},
		}

		Convey("Happy path with deadline", func() {
			originalItem := itemWithExp[[]byte]{[]byte("blah-blah"), now}

			blob, err := c.serializeItem(&originalItem)
			So(err, ShouldBeNil)

			item, err := c.deserializeItem(blob)
			So(err, ShouldBeNil)
			So(item.exp.Equal(now), ShouldBeTrue)
			So(item.val, ShouldResemble, originalItem.val)
		})

		Convey("Happy path without deadline", func() {
			originalItem := itemWithExp[[]byte]{[]byte("blah-blah"), time.Time{}}

			blob, err := c.serializeItem(&originalItem)
			So(err, ShouldBeNil)

			item, err := c.deserializeItem(blob)
			So(err, ShouldBeNil)
			So(item.exp.IsZero(), ShouldBeTrue)
			So(item.val, ShouldResemble, originalItem.val)
		})

		Convey("Marshal error", func() {
			fail := errors.New("failure")
			c.params.Marshal = func(item []byte) ([]byte, error) {
				return nil, fail
			}
			_, err := c.serializeItem(&itemWithExp[[]byte]{})
			So(err, ShouldEqual, fail)
		})

		Convey("Small buffer in Unmarshal", func() {
			_, err := c.deserializeItem([]byte{formatVersionByte, 0})
			So(err, ShouldErrLike, "buffer is too small")
		})

		Convey("Bad version in Unmarshal", func() {
			_, err := c.deserializeItem([]byte{formatVersionByte + 1, 0, 0, 0, 0, 0, 0, 0, 0})
			So(err, ShouldErrLike, "bad format version")
		})

		Convey("Unmarshal error", func() {
			fail := errors.New("failure")
			c.params.Unmarshal = func(blob []byte) ([]byte, error) {
				return nil, fail
			}

			blob, err := c.serializeItem(&itemWithExp[[]byte]{[]byte("blah-blah"), now})
			So(err, ShouldBeNil)

			_, err = c.deserializeItem(blob)
			So(err, ShouldEqual, fail)
		})
	})
}
