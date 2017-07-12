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

package coordinator

import (
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/logdog/common/storage/caching"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/memcache"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func testStorageCache(t *testing.T, compress bool) {
	t.Parallel()

	cloneItemsWithData := func(src []*caching.Item, data []byte) []*caching.Item {
		dst := make([]*caching.Item, len(src))
		for i, itm := range src {
			clone := *itm
			clone.Data = data
			dst[i] = &clone
		}
		return dst
	}

	Convey(`Testing storage cache in a testing envrionment`, t, func() {
		c := memory.Use(context.Background())
		c, tc := testclock.UseTime(c, testclock.TestTimeLocal)
		c, fb := featureBreaker.FilterMC(c, nil)

		var cache StorageCache
		if compress {
			cache.compressionThreshold = 1
		}

		items := []*caching.Item{
			{Schema: "test", Type: "type", Key: "foo", Data: []byte("foo")},
			{Schema: "test", Type: "type", Key: "bar", Data: []byte("bar")},
			{Schema: "test", Type: "othertype", Key: "foo", Data: []byte("foo2")},
			{Schema: "otherschema", Type: "othertype", Key: "foo", Data: []byte("foo3")},
		}

		Convey(`Can load those items into cache`, func() {
			cache.Put(c, time.Minute, items...)

			stats, err := memcache.Stats(c)
			So(err, ShouldBeNil)
			So(stats.Items, ShouldEqual, 4)

			Convey(`And retrieve those items from cache.`, func() {
				oit := cloneItemsWithData(items, []byte("junk"))
				cache.Get(c, oit...)
				So(oit, ShouldResemble, items)
			})

			Convey(`Returns nil data if memcache.GetMulti is broken.`, func() {
				fb.BreakFeatures(errors.New("test error"), "GetMulti")

				cache.Get(c, items...)
				So(items, ShouldResemble, cloneItemsWithData(items, nil))
			})
		})

		Convey(`Does not load items into cache if memcache.SetMulti is broken.`, func() {
			fb.BreakFeatures(errors.New("test error"), "SetMulti")

			cache.Put(c, time.Minute, items...)
			cache.Get(c, items...)
			So(items, ShouldResemble, cloneItemsWithData(items, nil))
		})

		Convey(`Get on missing item returns nil data.`, func() {
			cache.Get(c, items...)
			So(items, ShouldResemble, cloneItemsWithData(items, nil))
		})

		Convey(`Will replace existing item value.`, func() {
			cache.Put(c, time.Minute, items...)

			other := cloneItemsWithData(items, []byte("ohaithere"))
			cache.Put(c, time.Minute, other...)

			cache.Get(c, items...)
			So(items, ShouldResemble, other)
		})

		Convey(`Applies expiration (or lack thereof).`, func() {
			cache.Put(c, time.Minute, items[0])
			cache.Put(c, 0, items[1])

			// items[0]
			itm, err := memcache.GetKey(c, cache.mkCacheKey(items[0]))
			So(err, ShouldBeNil)
			So(itm.Key(), ShouldEqual, cache.mkCacheKey(items[0]))
			So(len(itm.Value()), ShouldNotEqual, 0)

			tc.Add(time.Minute + 1) // Expires items[0].
			_, err = memcache.GetKey(c, cache.mkCacheKey(items[0]))
			So(err, ShouldEqual, memcache.ErrCacheMiss)

			// items[1]
			itm, err = memcache.GetKey(c, cache.mkCacheKey(items[1]))
			So(err, ShouldBeNil)
			So(itm.Key(), ShouldEqual, cache.mkCacheKey(items[1]))
			So(len(itm.Value()), ShouldNotEqual, 0)
		})
	})
}

func TestStorageCacheWithoutCompression(t *testing.T) {
	testStorageCache(t, false)
}

func TestStorageCacheWithCompression(t *testing.T) {
	testStorageCache(t, true)
}
