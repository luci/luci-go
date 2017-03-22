// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package server

import (
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/filter/count"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMemcache(t *testing.T) {
	t.Parallel()

	Convey("with mocked memcache", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = memory.Use(ctx)
		ctx, counter := count.FilterMC(ctx)

		Convey("Memcache works (no per-request cache)", func() {
			cache := &Memcache{Namespace: "blah"}
			doTest(ctx, cache)
			So(counter.GetMulti.Total(), ShouldEqual, 5)
			So(counter.SetMulti.Total(), ShouldEqual, 2)
		})

		Convey("Memcache works (with per-request cache)", func() {
			cache := &Memcache{Namespace: "blah"}
			ctx = cache.UseRequestCache(ctx)
			doTest(ctx, cache)
			So(counter.GetMulti.Total(), ShouldEqual, 2)
			So(counter.SetMulti.Total(), ShouldEqual, 2)
		})
	})
}

func doTest(ctx context.Context, cache *Memcache) {
	// Cache miss.
	val, err := cache.Get(ctx, "key")
	So(err, ShouldBeNil)
	So(val, ShouldBeNil)

	So(cache.Set(ctx, "key_permanent", []byte("1"), 0), ShouldBeNil)
	So(cache.Set(ctx, "key_temp", []byte("2"), time.Minute), ShouldBeNil)

	// Cache hit.
	val, err = cache.Get(ctx, "key_permanent")
	So(err, ShouldBeNil)
	So(val, ShouldResemble, []byte("1"))

	val, err = cache.Get(ctx, "key_temp")
	So(err, ShouldBeNil)
	So(val, ShouldResemble, []byte("2"))

	// Expire one item.
	clock.Get(ctx).(testclock.TestClock).Add(2 * time.Minute)

	val, err = cache.Get(ctx, "key_permanent")
	So(err, ShouldBeNil)
	So(val, ShouldResemble, []byte("1"))

	// Expired!
	val, err = cache.Get(ctx, "key_temp")
	So(err, ShouldBeNil)
	So(val, ShouldBeNil)
}
