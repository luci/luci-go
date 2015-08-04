// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"testing"
	"time"

	mcS "github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestMemcache(t *testing.T) {
	t.Parallel()

	Convey("memcache", t, func() {
		now := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
		c, tc := testclock.UseTime(context.Background(), now)
		c = Use(c)
		mc := mcS.Get(c)

		Convey("implements MCSingleReadWriter", func() {
			Convey("Add", func() {
				itm := (mc.NewItem("sup").
					SetValue([]byte("cool")).
					SetExpiration(time.Second))
				So(mc.Add(itm), ShouldBeNil)
				Convey("which rejects objects already there", func() {
					So(mc.Add(itm), ShouldEqual, mcS.ErrNotStored)
				})
			})

			Convey("Get", func() {
				itm := &mcItem{
					key:        "sup",
					value:      []byte("cool"),
					expiration: time.Second,
				}
				So(mc.Add(itm), ShouldBeNil)

				testItem := &mcItem{
					key:   "sup",
					value: []byte("cool"),
					CasID: 1,
				}
				getItm := &mcItem{
					key: "sup",
				}
				So(mc.Get(getItm), ShouldBeNil)
				So(getItm, ShouldResemble, testItem)

				Convey("which can expire", func() {
					tc.Add(time.Second * 4)
					getItm := &mcItem{key: "sup"}
					So(mc.Get(getItm), ShouldEqual, mcS.ErrCacheMiss)
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
					So(mc.Add(itm), ShouldBeNil)

					So(mc.Delete("sup"), ShouldBeNil)

					So(mc.Get(mc.NewItem("sup")), ShouldEqual, mcS.ErrCacheMiss)
				})

				Convey("but not if it's not there", func() {
					So(mc.Delete("sup"), ShouldEqual, mcS.ErrCacheMiss)
				})
			})

			Convey("Set", func() {
				itm := &mcItem{
					key:        "sup",
					value:      []byte("cool"),
					expiration: time.Second,
				}
				So(mc.Add(itm), ShouldBeNil)

				itm.SetValue([]byte("newp"))
				So(mc.Set(itm), ShouldBeNil)

				testItem := &mcItem{
					key:   "sup",
					value: []byte("newp"),
					CasID: 2,
				}
				getItm := mc.NewItem("sup")
				So(mc.Get(getItm), ShouldBeNil)
				So(getItm, ShouldResemble, testItem)

				Convey("Flush works too", func() {
					mc.Flush()
					So(mc.Get(getItm), ShouldEqual, mcS.ErrCacheMiss)
				})
			})

			Convey("Increment", func() {
				val, err := mc.Increment("num", 7, 2)
				So(err, ShouldBeNil)
				So(val, ShouldEqual, 9)

				Convey("IncrementExisting", func() {
					val, err := mc.IncrementExisting("num", -2)
					So(err, ShouldBeNil)
					So(val, ShouldEqual, 7)

					val, err = mc.IncrementExisting("num", -100)
					So(err, ShouldBeNil)
					So(val, ShouldEqual, 0)

					_, err = mc.IncrementExisting("noexist", 2)
					So(err, ShouldEqual, mcS.ErrCacheMiss)

					So(mc.Set(mc.NewItem("text").SetValue([]byte("hello world, hooman!"))), ShouldBeNil)

					_, err = mc.IncrementExisting("text", 2)
					So(err.Error(), ShouldContainSubstring, "got invalid current value")
				})
			})

			Convey("CompareAndSwap", func() {
				itm := mcS.Item(&mcItem{
					key:        "sup",
					value:      []byte("cool"),
					expiration: time.Second * 2,
				})
				So(mc.Add(itm), ShouldBeNil)

				Convey("works after a Get", func() {
					itm = mc.NewItem("sup")
					So(mc.Get(itm), ShouldBeNil)
					So(itm.(*mcItem).CasID, ShouldEqual, 1)

					itm.SetValue([]byte("newp"))
					So(mc.CompareAndSwap(itm), ShouldBeNil)
				})

				Convey("but fails if you don't", func() {
					itm.SetValue([]byte("newp"))
					So(mc.CompareAndSwap(itm), ShouldEqual, mcS.ErrCASConflict)
				})

				Convey("and fails if the item is expired/gone", func() {
					tc.Add(3 * time.Second)
					itm.SetValue([]byte("newp"))
					So(mc.CompareAndSwap(itm), ShouldEqual, mcS.ErrNotStored)
				})
			})
		})

		Convey("check that the internal implementation is sane", func() {
			curTime := now
			err := mc.Add(&mcItem{
				key:        "sup",
				value:      []byte("cool"),
				expiration: time.Second * 2,
			})

			mc.Get(mc.NewItem("sup"))
			mc.Get(mc.NewItem("sup"))
			mc.Get(mc.NewItem("sup"))
			mc.Get(mc.NewItem("sup"))
			mc.Get(mc.NewItem("wot"))

			mci := mc.Raw().(*memcacheImpl)

			So(err, ShouldBeNil)
			stats, err := mc.Stats()
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

			getItm := mc.NewItem("sup")
			So(mc.Get(getItm), ShouldBeNil)
			So(len(mci.data.items), ShouldEqual, 1)
			So(mci.data.casID, ShouldEqual, 1)

			testItem := &mcItem{
				key:   "sup",
				value: []byte("cool"),
				CasID: 1,
			}
			So(getItm, ShouldResemble, testItem)
		})

	})
}
