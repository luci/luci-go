// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package unsafe

import (
	"testing"

	"appengine/memcache"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnsafeMemcacheItem(t *testing.T) {
	Convey("unsafe memcache", t, func() {
		// kinda a bogus test, but it forces the init() to run, which checks the
		// field alignment and should panic bigtime if things are wrong.
		itm := &memcache.Item{}
		So(MCGetCasID(itm), ShouldEqual, 0)
		MCSetCasID(itm, 10)
		So(MCGetCasID(itm), ShouldEqual, 10)
	})
}
