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
	"errors"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var testingCache = caching.RegisterLRUCache(0)

func TestCache(t *testing.T) {
	t.Parallel()

	Convey("With fake time", t, func() {
		ctx := context.Background()
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
			global := &blobCache{lru: lru.New(0)}

			ctx = caching.WithGlobalCache(ctx, func(ns string) caching.BlobCache {
				if ns != c.GlobalNamespace {
					panic("wrong namespace")
				}
				return global
			})

			Convey("Getting from the global cache", func() {
				// The global cache is empty.
				So(global.lru.Len(), ShouldEqual, 0)

				// Create an item.
				item, err := c.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 1)

				// It is in the global cache now.
				So(global.lru.Len(), ShouldEqual, 1)

				// Clear the local cache.
				ctx = caching.WithEmptyProcessCache(ctx)

				// Grab the item again. Will be fetched from the global cache.
				item, err = c.GetOrCreate(ctx, "item", getter)
				So(err, ShouldBeNil)
				So(item, ShouldResemble, value)
				So(calls, ShouldEqual, 1) // no new calls
			})

			Convey("Broken global cache is ignored", func() {
				global.err = errors.New("broken!")

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
			originalItem := []byte("blah-blah")

			blob, err := c.serializeItem(originalItem, now)
			So(err, ShouldBeNil)

			item, expTS, err := c.deserializeItem(blob)
			So(err, ShouldBeNil)
			So(expTS.Equal(now), ShouldBeTrue)
			So(item, ShouldResemble, originalItem)
		})

		Convey("Happy path without deadline", func() {
			originalItem := []byte("blah-blah")

			blob, err := c.serializeItem(originalItem, time.Time{})
			So(err, ShouldBeNil)

			item, expTS, err := c.deserializeItem(blob)
			So(err, ShouldBeNil)
			So(expTS.IsZero(), ShouldBeTrue)
			So(item, ShouldResemble, originalItem)
		})

		Convey("Marshal error", func() {
			fail := errors.New("failure")
			c.Marshal = func(item interface{}) ([]byte, error) {
				return nil, fail
			}
			_, err := c.serializeItem(nil, time.Time{})
			So(err, ShouldEqual, fail)
		})

		Convey("Small buffer in Unmarshal", func() {
			_, _, err := c.deserializeItem([]byte{formatVersionByte, 0})
			So(err, ShouldErrLike, "buffer is too small")
		})

		Convey("Bad version in Unmarshal", func() {
			_, _, err := c.deserializeItem([]byte{formatVersionByte + 1, 0, 0, 0, 0, 0, 0, 0, 0})
			So(err, ShouldErrLike, "bad format version")
		})

		Convey("Unmarshal error", func() {
			fail := errors.New("failure")
			c.Unmarshal = func(blob []byte) (interface{}, error) {
				return nil, fail
			}

			blob, err := c.serializeItem([]byte("blah-blah"), now)
			So(err, ShouldBeNil)

			_, _, err = c.deserializeItem(blob)
			So(err, ShouldEqual, fail)
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

// blobCache implements caching.BlobCache on top of lru.Cache (for testing).
type blobCache struct {
	lru *lru.Cache
	err error
}

func (b *blobCache) Get(c context.Context, key string) ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	item, ok := b.lru.Get(c, key)
	if !ok {
		return nil, caching.ErrCacheMiss
	}
	return item.([]byte), nil
}

func (b *blobCache) Set(c context.Context, key string, value []byte, exp time.Duration) error {
	if b.err != nil {
		return b.err
	}
	b.lru.Put(c, key, value, exp)
	return nil
}
