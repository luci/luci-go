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
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var testingCache = caching.RegisterLRUCache(0)

func TestCache(t *testing.T) {
	t.Parallel()

	Convey("With fake time", t, func() {
		ctx := context.Background()
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(12345)))
		ctx, tc := testclock.UseTime(ctx, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC))
		ctx = caching.WithEmptyProcessCache(ctx)

		c := Cache{
			ProcessLRUCache: testingCache,
			GlobalNamespace: "namespace",
			Marshal: func(item interface{}) ([]byte, error) {
				return item.([]byte), nil
			},
			Unmarshal: func(blob []byte) (interface{}, error) {
				return blob, nil
			},
		}

		calls := 0
		value := []byte("value")
		anotherValue := []byte("anotherValue")

		getter := func() (interface{}, time.Duration, error) {
			calls++
			return value, time.Hour, nil
		}

		Convey("Without global cache", func() {
			item, err := c.GetOrCreate(ctx, "item", getter)
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)
			So(calls, ShouldEqual, 1)

			tc.Add(59 * time.Minute)

			item, err = c.GetOrCreate(ctx, "item", getter)
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)
			So(calls, ShouldEqual, 1) // no new calls

			tc.Add(2 * time.Minute) // cached item expires

			item, err = c.GetOrCreate(ctx, "item", getter)
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)
			So(calls, ShouldEqual, 2) // new call!
		})

		Convey("With global cache", func() {
			global := &cachingtest.BlobCache{LRU: lru.New(0)}
			ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
				c.GlobalNamespace: global,
			})

			Convey("Getting from the global cache", func() {
				// The global cache is empty.
				So(global.LRU.Len(), ShouldEqual, 0)

				// Create an item.
				item, err := c.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 1)

				// It is in the global cache now.
				So(global.LRU.Len(), ShouldEqual, 1)

				// Clear the local cache.
				ctx = caching.WithEmptyProcessCache(ctx)

				// Grab the item again. Will be fetched from the global cache.
				item, err = c.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 1) // no new calls
			})

			Convey("Broken global cache is ignored", func() {
				global.Err = errors.New("broken")

				// Create an item.
				item, err := c.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 1)

				// Clear the local cache.
				ctx = caching.WithEmptyProcessCache(ctx)

				// Grab the item again. Will be recreated again, since the global cache
				// is broken.
				item, err = c.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 2) // new call!
			})
		})

		Convey("Never expiring item", func() {
			item, err := c.GetOrCreate(ctx, "item", func() (interface{}, time.Duration, error) {
				return value, 0, nil
			})
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)

			tc.Add(100 * time.Hour)

			item, err = c.GetOrCreate(ctx, "item", func() (interface{}, time.Duration, error) {
				return nil, 0, errors.New("must not be called")
			})
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)
		})

		Convey("WithMinTTL works", func() {
			item, err := c.GetOrCreate(ctx, "item", func() (interface{}, time.Duration, error) {
				return value, time.Hour, nil
			})
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)

			tc.Add(50 * time.Minute)

			// 9 min minTTL is still ok.
			item, err = c.GetOrCreate(ctx, "item", func() (interface{}, time.Duration, error) {
				return nil, 0, errors.New("must not be called")
			}, WithMinTTL(9*time.Minute))
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)

			// But 10 min is not and the item is refreshed.
			item, err = c.GetOrCreate(ctx, "item", func() (interface{}, time.Duration, error) {
				return anotherValue, time.Hour, nil
			}, WithMinTTL(10*time.Minute))
			So(err, ShouldBeNil)
			So(item, ShouldResemble, anotherValue)
		})

		Convey("ErrCantSatisfyMinTTL", func() {
			_, err := c.GetOrCreate(ctx, "item", func() (interface{}, time.Duration, error) {
				return value, time.Minute, nil
			}, WithMinTTL(2*time.Minute))
			So(err, ShouldEqual, ErrCantSatisfyMinTTL)
		})

		oneRandomizedTrial := func(now, threshold time.Duration) (cacheHit bool) {
			// Reset state (except RNG).
			ctx := caching.WithEmptyProcessCache(ctx)
			ctx, tc := testclock.UseTime(ctx, time.Date(2017, time.January, 1, 0, 0, 0, 0, time.UTC))

			// Put the item in the cache.
			item, err := c.GetOrCreate(ctx, "item", func() (interface{}, time.Duration, error) {
				return value, time.Hour, nil
			})
			So(err, ShouldBeNil)
			So(item, ShouldResemble, value)

			tc.Add(now)

			// Grab it (or trigger a refresh if randomly expired).
			item, err = c.GetOrCreate(ctx, "item", func() (interface{}, time.Duration, error) {
				return anotherValue, time.Hour, nil
			}, WithRandomizedExpiration(threshold))
			So(err, ShouldBeNil)
			return bytes.Equal(item.([]byte), value)
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

		c := Cache{
			Marshal: func(item interface{}) ([]byte, error) {
				return item.([]byte), nil
			},
			Unmarshal: func(blob []byte) (interface{}, error) {
				return blob, nil
			},
		}

		Convey("Happy path with deadline", func() {
			originalItem := itemWithExp{[]byte("blah-blah"), now}

			blob, err := c.serializeItem(&originalItem)
			So(err, ShouldBeNil)

			item, err := c.deserializeItem(blob)
			So(err, ShouldBeNil)
			So(item.exp.Equal(now), ShouldBeTrue)
			So(item.val, ShouldResemble, originalItem.val)
		})

		Convey("Happy path without deadline", func() {
			originalItem := itemWithExp{[]byte("blah-blah"), time.Time{}}

			blob, err := c.serializeItem(&originalItem)
			So(err, ShouldBeNil)

			item, err := c.deserializeItem(blob)
			So(err, ShouldBeNil)
			So(item.exp.IsZero(), ShouldBeTrue)
			So(item.val, ShouldResemble, originalItem.val)
		})

		Convey("Marshal error", func() {
			fail := errors.New("failure")
			c.Marshal = func(item interface{}) ([]byte, error) {
				return nil, fail
			}
			_, err := c.serializeItem(&itemWithExp{})
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
			c.Unmarshal = func(blob []byte) (interface{}, error) {
				return nil, fail
			}

			blob, err := c.serializeItem(&itemWithExp{[]byte("blah-blah"), now})
			So(err, ShouldBeNil)

			_, err = c.deserializeItem(blob)
			So(err, ShouldEqual, fail)
		})
	})
}
