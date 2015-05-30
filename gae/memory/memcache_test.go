// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"infra/gae/libs/wrapper"
	"infra/gae/libs/wrapper/unsafe"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"

	"appengine/memcache"
)

func TestMemcache(t *testing.T) {
	t.Parallel()

	Convey("memcache", t, func() {
		now := time.Now()
		timeNow := func(context.Context) time.Time {
			ret := now
			now = now.Add(time.Second)
			return ret
		}
		c := Use(wrapper.SetTimeNowFactory(context.Background(), timeNow))
		mc := wrapper.GetMC(c)
		mci := mc.(*memcacheImpl)

		Convey("implements MCSingleReadWriter", func() {
			Convey("Add", func() {
				itm := &memcache.Item{
					Key:        "sup",
					Value:      []byte("cool"),
					Expiration: time.Second,
				}
				err := mc.Add(itm)
				So(err, ShouldBeNil)
				Convey("which rejects objects already there", func() {
					err := mc.Add(itm)
					So(err, ShouldEqual, memcache.ErrNotStored)
				})

				Convey("which can be broken intentionally", func() {
					mci.BreakFeatures(nil, "Add")
					err := mc.Add(itm)
					So(err, ShouldEqual, memcache.ErrServerError)
				})
			})

			Convey("Get", func() {
				itm := &memcache.Item{
					Key:        "sup",
					Value:      []byte("cool"),
					Expiration: time.Second,
				}
				err := mc.Add(itm)
				So(err, ShouldBeNil)

				testItem := &memcache.Item{
					Key:   "sup",
					Value: []byte("cool"),
				}
				unsafe.MCSetCasID(testItem, 1)
				i, err := mc.Get("sup")
				So(err, ShouldBeNil)
				So(i, ShouldResemble, testItem)

				Convey("which can expire", func() {
					now = now.Add(time.Second * 4)
					i, err := mc.Get("sup")
					So(err, ShouldEqual, memcache.ErrCacheMiss)
					So(i, ShouldBeNil)
				})

				Convey("which can be broken intentionally", func() {
					mci.BreakFeatures(nil, "Get")
					_, err := mc.Get("sup")
					So(err, ShouldEqual, memcache.ErrServerError)
				})
			})

			Convey("Delete", func() {
				Convey("works if it's there", func() {
					itm := &memcache.Item{
						Key:        "sup",
						Value:      []byte("cool"),
						Expiration: time.Second,
					}
					err := mc.Add(itm)
					So(err, ShouldBeNil)

					err = mc.Delete("sup")
					So(err, ShouldBeNil)

					i, err := mc.Get("sup")
					So(err, ShouldEqual, memcache.ErrCacheMiss)
					So(i, ShouldBeNil)
				})

				Convey("but not if it's not there", func() {
					err := mc.Delete("sup")
					So(err, ShouldEqual, memcache.ErrCacheMiss)
				})

				Convey("and can be broken", func() {
					mci.BreakFeatures(nil, "Delete")
					err := mc.Delete("sup")
					So(err, ShouldEqual, memcache.ErrServerError)
				})
			})

			Convey("Set", func() {
				itm := &memcache.Item{
					Key:        "sup",
					Value:      []byte("cool"),
					Expiration: time.Second,
				}
				err := mc.Add(itm)
				So(err, ShouldBeNil)

				itm.Value = []byte("newp")
				err = mc.Set(itm)
				So(err, ShouldBeNil)

				testItem := &memcache.Item{
					Key:   "sup",
					Value: []byte("newp"),
				}
				unsafe.MCSetCasID(testItem, 2)
				i, err := mc.Get("sup")
				So(err, ShouldBeNil)
				So(i, ShouldResemble, testItem)

				Convey("and can be broken", func() {
					mci.BreakFeatures(nil, "Set")
					err := mc.Set(itm)
					So(err, ShouldEqual, memcache.ErrServerError)
				})
			})

			Convey("CompareAndSwap", func() {
				itm := &memcache.Item{
					Key:        "sup",
					Value:      []byte("cool"),
					Expiration: time.Second * 2,
				}
				err := mc.Add(itm)
				So(err, ShouldBeNil)

				Convey("works after a Get", func() {
					itm, err = mc.Get("sup")
					So(err, ShouldBeNil)
					So(unsafe.MCGetCasID(itm), ShouldEqual, 1)

					itm.Value = []byte("newp")
					err = mc.CompareAndSwap(itm)
					So(err, ShouldBeNil)
				})

				Convey("but fails if you don't", func() {
					itm.Value = []byte("newp")
					err = mc.CompareAndSwap(itm)
					So(err, ShouldEqual, memcache.ErrCASConflict)
				})

				Convey("and fails if the item is expired/gone", func() {
					// run the clock forward
					wrapper.GetTimeNow(c)
					wrapper.GetTimeNow(c)
					itm.Value = []byte("newp")
					err = mc.CompareAndSwap(itm)
					So(err, ShouldEqual, memcache.ErrNotStored)
				})

				Convey("and can be broken", func() {
					mci.BreakFeatures(nil, "CompareAndSwap")
					err = mc.CompareAndSwap(itm)
					So(err, ShouldEqual, memcache.ErrServerError)
				})
			})
		})

		Convey("check that the internal implementation is sane", func() {
			curTime := now
			err := mc.Add(&memcache.Item{
				Key:        "sup",
				Value:      []byte("cool"),
				Expiration: time.Second * 2,
			})

			So(err, ShouldBeNil)
			So(len(mci.data.items), ShouldEqual, 1)
			So(mci.data.casID, ShouldEqual, 1)
			So(mci.data.items["sup"], ShouldResemble, &unsafe.Item{
				Key:        "sup",
				Value:      []byte("cool"),
				CasID:      1,
				Expiration: time.Duration(curTime.Add(time.Second * 2).UnixNano()),
			})

			el, err := mc.Get("sup")
			So(err, ShouldBeNil)
			So(len(mci.data.items), ShouldEqual, 1)
			So(mci.data.casID, ShouldEqual, 1)

			testItem := &memcache.Item{
				Key:   "sup",
				Value: []byte("cool"),
			}
			unsafe.MCSetCasID(testItem, 1)
			So(el, ShouldResemble, testItem)
		})

	})
}
