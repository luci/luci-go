// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dummy

import (
	"testing"

	infoS "github.com/luci/gae/service/info"
	mcS "github.com/luci/gae/service/memcache"
	rdsS "github.com/luci/gae/service/rawdatastore"
	tqS "github.com/luci/gae/service/taskqueue"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
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
			So(mcS.Get(c), ShouldBeNil)
			So(tqS.Get(c), ShouldBeNil)
			So(infoS.Get(c), ShouldBeNil)
		})

		Convey("RawDatastore", func() {
			c = rdsS.Set(c, RawDatastore())
			So(rdsS.Get(c), ShouldNotBeNil)
			So(func() {
				defer p()
				rdsS.Get(c).NewKey("", "", 1, nil)
			}, ShouldPanicWith, "dummy: method RawDatastore.NewKey is not implemented")
		})

		Convey("Memcache", func() {
			c = mcS.Set(c, Memcache())
			So(mcS.Get(c), ShouldNotBeNil)
			So(func() {
				defer p()
				mcS.Get(c).Add(nil)
			}, ShouldPanicWith, "dummy: method Memcache.Add is not implemented")
		})

		Convey("TaskQueue", func() {
			c = tqS.Set(c, TaskQueue())
			So(tqS.Get(c), ShouldNotBeNil)
			So(func() {
				defer p()
				tqS.Get(c).Purge("")
			}, ShouldPanicWith, "dummy: method TaskQueue.Purge is not implemented")
		})

		Convey("Info", func() {
			c = infoS.Set(c, Info())
			So(infoS.Get(c), ShouldNotBeNil)
			So(func() {
				defer p()
				infoS.Get(c).Datacenter()
			}, ShouldPanicWith, "dummy: method Info.Datacenter is not implemented")
		})
	})
}
