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

package memory

import (
	"testing"
	"time"

	"go.chromium.org/gae/service/info"
	mc "go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMemcache(t *testing.T) {
	t.Parallel()

	Convey("memcache", t, func() {
		now := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
		c, tc := testclock.UseTime(context.Background(), now)
		c = Use(c)

		Convey("implements MCSingleReadWriter", func() {
			Convey("Add", func() {
				itm := (mc.NewItem(c, "sup").
					SetValue([]byte("cool")).
					SetExpiration(time.Second))
				So(mc.Add(c, itm), ShouldBeNil)
				Convey("which rejects objects already there", func() {
					So(mc.Add(c, itm), ShouldEqual, mc.ErrNotStored)
				})
			})

			Convey("Get", func() {
				itm := &mcItem{
					key:        "sup",
					value:      []byte("cool"),
					expiration: time.Second,
				}
				So(mc.Add(c, itm), ShouldBeNil)

				testItem := &mcItem{
					key:   "sup",
					value: []byte("cool"),
					CasID: 1,
				}
				getItm, err := mc.GetKey(c, "sup")
				So(err, ShouldBeNil)
				So(getItm, ShouldResemble, testItem)

				Convey("which can expire", func() {
					tc.Add(time.Second * 4)
					getItm, err := mc.GetKey(c, "sup")
					So(err, ShouldEqual, mc.ErrCacheMiss)
					So(getItm, ShouldResemble, &mcItem{key: "sup"})
				})
			})

			Convey("Delete", func() {
				Convey("works if it's there", func() {
					itm := &mcItem{
						key:        "sup",
						value:      []byte("cool"),
						expiration: time.Second,
					}
					So(mc.Add(c, itm), ShouldBeNil)

					So(mc.Delete(c, "sup"), ShouldBeNil)

					_, err := mc.GetKey(c, "sup")
					So(err, ShouldEqual, mc.ErrCacheMiss)
				})

				Convey("but not if it's not there", func() {
					So(mc.Delete(c, "sup"), ShouldEqual, mc.ErrCacheMiss)
				})
			})

			Convey("Set", func() {
				itm := &mcItem{
					key:        "sup",
					value:      []byte("cool"),
					expiration: time.Second,
				}
				So(mc.Add(c, itm), ShouldBeNil)

				itm.SetValue([]byte("newp"))
				So(mc.Set(c, itm), ShouldBeNil)

				testItem := &mcItem{
					key:   "sup",
					value: []byte("newp"),
					CasID: 2,
				}
				getItm, err := mc.GetKey(c, "sup")
				So(err, ShouldBeNil)
				So(getItm, ShouldResemble, testItem)

				Convey("Flush works too", func() {
					So(mc.Flush(c), ShouldBeNil)
					_, err := mc.GetKey(c, "sup")
					So(err, ShouldEqual, mc.ErrCacheMiss)
				})
			})

			Convey("Set (nil) is equivalent to Set([]byte{})", func() {
				So(mc.Set(c, mc.NewItem(c, "bob")), ShouldBeNil)

				bob, err := mc.GetKey(c, "bob")
				So(err, ShouldBeNil)
				So(bob.Value(), ShouldResemble, []byte{})
			})

			Convey("Increment", func() {
				val, err := mc.Increment(c, "num", 7, 2)
				So(err, ShouldBeNil)
				So(val, ShouldEqual, 9)

				Convey("IncrementExisting", func() {
					val, err := mc.IncrementExisting(c, "num", -2)
					So(err, ShouldBeNil)
					So(val, ShouldEqual, 7)

					val, err = mc.IncrementExisting(c, "num", -100)
					So(err, ShouldBeNil)
					So(val, ShouldEqual, 0)

					_, err = mc.IncrementExisting(c, "noexist", 2)
					So(err, ShouldEqual, mc.ErrCacheMiss)

					So(mc.Set(c, mc.NewItem(c, "text").SetValue([]byte("hello world, hooman!"))), ShouldBeNil)

					_, err = mc.IncrementExisting(c, "text", 2)
					So(err.Error(), ShouldContainSubstring, "got invalid current value")
				})
			})

			Convey("CompareAndSwap", func() {
				itm := mc.Item(&mcItem{
					key:        "sup",
					value:      []byte("cool"),
					expiration: time.Second * 2,
				})
				So(mc.Add(c, itm), ShouldBeNil)

				Convey("works after a Get", func() {
					itm, err := mc.GetKey(c, "sup")
					So(err, ShouldBeNil)
					So(itm.(*mcItem).CasID, ShouldEqual, 1)

					itm.SetValue([]byte("newp"))
					So(mc.CompareAndSwap(c, itm), ShouldBeNil)
				})

				Convey("but fails if you don't", func() {
					itm.SetValue([]byte("newp"))
					So(mc.CompareAndSwap(c, itm), ShouldEqual, mc.ErrCASConflict)
				})

				Convey("and fails if the item is expired/gone", func() {
					tc.Add(3 * time.Second)
					itm.SetValue([]byte("newp"))
					So(mc.CompareAndSwap(c, itm), ShouldEqual, mc.ErrNotStored)
				})
			})
		})

		Convey("check that the internal implementation is sane", func() {
			curTime := now
			err := mc.Add(c, &mcItem{
				key:        "sup",
				value:      []byte("cool"),
				expiration: time.Second * 2,
			})

			for i := 0; i < 4; i++ {
				_, err := mc.GetKey(c, "sup")
				So(err, ShouldBeNil)
			}
			_, err = mc.GetKey(c, "wot")
			So(err, ShouldErrLike, mc.ErrCacheMiss)

			mci := mc.Raw(c).(*memcacheImpl)

			stats, err := mc.Stats(c)
			So(err, ShouldBeNil)
			So(stats.Items, ShouldEqual, 1)
			So(stats.Bytes, ShouldEqual, 4)
			So(stats.Hits, ShouldEqual, 4)
			So(stats.Misses, ShouldEqual, 1)
			So(stats.ByteHits, ShouldEqual, 4*4)
			So(mci.data.casID, ShouldEqual, 1)
			So(mci.data.items["sup"], ShouldResemble, &mcDataItem{
				value:      []byte("cool"),
				expiration: curTime.Add(time.Second * 2).Truncate(time.Second),
				casID:      1,
			})

			getItm, err := mc.GetKey(c, "sup")
			So(err, ShouldBeNil)
			So(len(mci.data.items), ShouldEqual, 1)
			So(mci.data.casID, ShouldEqual, 1)

			testItem := &mcItem{
				key:   "sup",
				value: []byte("cool"),
				CasID: 1,
			}
			So(getItm, ShouldResemble, testItem)
		})

		Convey("When adding an item to an unset namespace", func() {
			So(info.GetNamespace(c), ShouldEqual, "")

			item := mc.NewItem(c, "foo").SetValue([]byte("heya"))
			So(mc.Set(c, item), ShouldBeNil)

			Convey("The item can be retrieved from the unset namespace.", func() {
				got, err := mc.GetKey(c, "foo")
				So(err, ShouldBeNil)
				So(got.Value(), ShouldResemble, []byte("heya"))
			})

			Convey("The item can be retrieved from a set, empty namespace.", func() {
				// Now test with empty namespace.
				c = info.MustNamespace(c, "")

				got, err := mc.GetKey(c, "foo")
				So(err, ShouldBeNil)
				So(got.Value(), ShouldResemble, []byte("heya"))
			})
		})
	})
}
