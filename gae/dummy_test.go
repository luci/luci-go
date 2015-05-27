// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

import (
	"math/rand"
	"testing"
	"time"

	"golang.org/x/net/context"

	"appengine/memcache"

	. "github.com/smartystreets/goconvey/convey"
)

func TestContextAccess(t *testing.T) {
	Convey("Context Access", t, func() {
		c := context.Background()

		Convey("blank", func() {
			So(GetDS(c), ShouldBeNil)
			So(GetMC(c), ShouldBeNil)
			So(GetTQ(c), ShouldBeNil)
			So(GetGI(c), ShouldBeNil)

			now := time.Now()
			So(GetTimeNow(c), ShouldHappenOnOrAfter, now)
		})

		Convey("DS", func() {
			c = SetDS(c, DummyDS())
			So(GetDS(c), ShouldNotBeNil)
			So(func() { GetDS(c).Kind(nil) }, ShouldPanic)
		})

		Convey("MC", func() {
			c = SetMC(c, DummyMC())
			So(GetMC(c), ShouldNotBeNil)
			So(func() { GetMC(c).InflateCodec(memcache.Codec{}) }, ShouldPanic)
		})

		Convey("TQ", func() {
			c = SetTQ(c, DummyTQ())
			So(GetTQ(c), ShouldNotBeNil)
			So(func() { GetTQ(c).Purge("") }, ShouldPanic)
		})

		Convey("GI", func() {
			c = SetGI(c, DummyGI())
			So(GetGI(c), ShouldNotBeNil)
			So(func() { GetGI(c).Datacenter() }, ShouldPanic)
		})

		Convey("QY", func() {
			q := DummyQY()
			So(func() { q.Distinct() }, ShouldPanic)
		})

		Convey("TimeNow", func() {
			thing := time.Date(2000, time.August, 20, 0, 0, 0, 0, time.UTC)
			c = SetTimeNow(c, &thing)
			So(GetTimeNow(c), ShouldResemble, thing)

			Convey("MathRand", func() {
				r := rand.New(rand.NewSource(thing.UnixNano()))
				i := r.Int()

				// when it's unset it picks TimeNow every time
				So(GetMathRand(c).Int(), ShouldEqual, i)
				So(GetMathRand(c).Int(), ShouldEqual, i)

				// But we could set it to something concrete to have it persist.
				c = SetMathRand(c, rand.New(rand.NewSource(thing.UnixNano())))
				r = rand.New(rand.NewSource(thing.UnixNano()))
				So(GetMathRand(c).Int(), ShouldEqual, r.Int())
				So(GetMathRand(c).Int(), ShouldEqual, r.Int())
			})

			c = SetTimeNow(c, nil)
			now := time.Now()
			So(GetTimeNow(c), ShouldHappenOnOrAfter, now)
		})

	})
}
