// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"testing"

	"github.com/luci/gae/service/module"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestModule(t *testing.T) {
	Convey("NumInstances", t, func() {
		c := Use(context.Background())
		m := module.Get(c)

		i, err := m.NumInstances("foo", "bar")
		So(i, ShouldEqual, 1)
		So(err, ShouldBeNil)

		So(m.SetNumInstances("foo", "bar", 42), ShouldBeNil)
		i, err = m.NumInstances("foo", "bar")
		So(i, ShouldEqual, 42)
		So(err, ShouldBeNil)

		i, err = m.NumInstances("foo", "baz")
		So(i, ShouldEqual, 1)
		So(err, ShouldBeNil)
	})
}
