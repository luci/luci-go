// Copyright 2015 The LUCI Authors.
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

package flex

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

func testStorageCache(t *testing.T, compress bool) {
	t.Parallel()

	Convey(`Testing storage cache in a testing environment`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = caching.WithEmptyProcessCache(c)
		cache := StorageCache{}
		if compress {
			cache.compressionThreshold = 1
		}

		items := []struct {
			k storage.CacheKey
			v []byte
		}{
			{storage.CacheKey{"test", "type", "foo"}, []byte("foo")},
			{storage.CacheKey{"test", "type", "bar"}, []byte("bar")},
			{storage.CacheKey{"test", "othertype", "foo"}, []byte("foo2")},
			{storage.CacheKey{"otherschema", "othertype", "foo"}, []byte("foo3")},
		}

		Convey(`Can load those items into cache`, func() {
			for _, it := range items {
				cache.Put(c, it.k, it.v, time.Minute)
			}
			So(storageCache.LRU(c).Len(), ShouldEqual, 4)

			for _, it := range items {
				v, ok := cache.Get(c, it.k)
				So(ok, ShouldBeTrue)
				So(v, ShouldResemble, it.v)
			}
		})

		Convey(`Get on missing item returns false.`, func() {
			_, ok := cache.Get(c, items[0].k)
			So(ok, ShouldBeFalse)
		})

		Convey(`Will replace existing item value.`, func() {
			cache.Put(c, items[0].k, items[0].v, time.Minute)
			v, ok := cache.Get(c, items[0].k)
			So(ok, ShouldBeTrue)
			So(v, ShouldResemble, items[0].v)

			cache.Put(c, items[0].k, []byte("ohai"), time.Minute)
			v, ok = cache.Get(c, items[0].k)
			So(ok, ShouldBeTrue)
			So(v, ShouldResemble, []byte("ohai"))
		})

		Convey(`Applies expiration (or lack thereof).`, func() {
			cache.Put(c, items[0].k, items[0].v, time.Minute)
			cache.Put(c, items[1].k, items[1].v, -1)

			v, has := cache.Get(c, items[0].k)
			So(has, ShouldBeTrue)
			So(v, ShouldResemble, items[0].v)

			v, has = cache.Get(c, items[1].k)
			So(has, ShouldBeTrue)
			So(v, ShouldResemble, items[1].v)

			tc.Add(time.Minute + 1) // Expires items[0].

			v, has = cache.Get(c, items[0].k)
			So(has, ShouldBeFalse)

			v, has = cache.Get(c, items[1].k)
			So(has, ShouldBeTrue)
			So(v, ShouldResemble, items[1].v)
		})
	})
}

func TestStorageCacheWithoutCompression(t *testing.T) {
	testStorageCache(t, false)
}

func TestStorageCacheWithCompression(t *testing.T) {
	testStorageCache(t, true)
}
