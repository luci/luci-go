// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mathrand

import (
	"math/rand"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func Test(t *testing.T) {
	t.Parallel()

	Convey("test mathrand", t, func() {
		now := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
		c, _ := testclock.UseTime(context.Background(), now)

		// Note that the non-randomness below is because time is fixed at the
		// top of the outer test function. Normally it would evolve with time.
		Convey("unset", func() {
			r := rand.New(rand.NewSource(now.UnixNano()))
			i := r.Int()
			So(Get(c).Int(), ShouldEqual, i)
			So(Get(c).Int(), ShouldEqual, i)
		})

		Convey("set persistance", func() {
			c = Set(c, rand.New(rand.NewSource(now.UnixNano())))
			r := rand.New(rand.NewSource(now.UnixNano()))
			So(Get(c).Int(), ShouldEqual, r.Int())
			So(Get(c).Int(), ShouldEqual, r.Int())
		})

		Convey("nil set", func() {
			c = Set(c, nil)
			r := rand.New(rand.NewSource(now.UnixNano()))
			i := r.Int()
			So(Get(c).Int(), ShouldEqual, i)
			So(Get(c).Int(), ShouldEqual, i)
		})
	})
}
