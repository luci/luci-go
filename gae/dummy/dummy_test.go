// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dummy

import (
	"golang.org/x/net/context"
	"testing"

	"infra/gae/libs/gae"

	. "github.com/smartystreets/goconvey/convey"
)

func TestContextAccess(t *testing.T) {
	t.Parallel()

	// p is a function which recovers an error and then immediately panics with
	// the contained string. It's defer'd in each test so that we can use the
	// ShouldPanicWith assertion (which does an == comparison and not
	// a reflect.DeepEquals comparison).
	p := func() { panic(recover().(error).Error()) }

	Convey("Context Access", t, func() {
		c := context.Background()

		Convey("blank", func() {
			So(gae.GetMC(c), ShouldBeNil)
			So(gae.GetTQ(c), ShouldBeNil)
			So(gae.GetGI(c), ShouldBeNil)
		})

		Convey("RDS", func() {
			c = gae.SetRDS(c, RDS())
			So(gae.GetRDS(c), ShouldNotBeNil)
			So(func() {
				defer p()
				gae.GetRDS(c).NewKey("", "", 1, nil)
			}, ShouldPanicWith, "dummy: method RawDatastore.NewKey is not implemented")
		})

		Convey("MC", func() {
			c = gae.SetMC(c, MC())
			So(gae.GetMC(c), ShouldNotBeNil)
			So(func() {
				defer p()
				gae.GetMC(c).Add(nil)
			}, ShouldPanicWith, "dummy: method Memcache.Add is not implemented")
		})

		Convey("TQ", func() {
			c = gae.SetTQ(c, TQ())
			So(gae.GetTQ(c), ShouldNotBeNil)
			So(func() {
				defer p()
				gae.GetTQ(c).Purge("")
			}, ShouldPanicWith, "dummy: method TaskQueue.Purge is not implemented")
		})

		Convey("GI", func() {
			c = gae.SetGI(c, GI())
			So(gae.GetGI(c), ShouldNotBeNil)
			So(func() {
				defer p()
				gae.GetGI(c).Datacenter()
			}, ShouldPanicWith, "dummy: method GlobalInfo.Datacenter is not implemented")
		})

		Convey("QY", func() {
			q := QY()
			So(func() {
				defer p()
				q.Distinct()
			}, ShouldPanicWith, "dummy: method DSQuery.Distinct is not implemented")
		})
	})
}
