// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"math/rand"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMathRand(t *testing.T) {
	t.Parallel()

	Convey("test mathrand", t, func() {
		now := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
		c, _ := testclock.UseTime(context.Background(), now)

		// Note that the non-randomness below is because time is fixed at the
		// top of the outer test function. Normally it would evolve with time.
		Convey("unset", func() {
			r := rand.New(rand.NewSource(now.UnixNano()))
			i := r.Int()
			So(GetMathRand(c).Int(), ShouldEqual, i)
			So(GetMathRand(c).Int(), ShouldEqual, i)
		})

		Convey("set persistance", func() {
			c = SetMathRand(c, rand.New(rand.NewSource(now.UnixNano())))
			r := rand.New(rand.NewSource(now.UnixNano()))
			So(GetMathRand(c).Int(), ShouldEqual, r.Int())
			So(GetMathRand(c).Int(), ShouldEqual, r.Int())
		})

		Convey("nil set", func() {
			c = SetMathRand(c, nil)
			r := rand.New(rand.NewSource(now.UnixNano()))
			i := r.Int()
			So(GetMathRand(c).Int(), ShouldEqual, i)
			So(GetMathRand(c).Int(), ShouldEqual, i)
		})
	})
}
