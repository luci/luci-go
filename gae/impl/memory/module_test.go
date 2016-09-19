// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

		i, err := module.NumInstances(c, "foo", "bar")
		So(i, ShouldEqual, 1)
		So(err, ShouldBeNil)

		So(module.SetNumInstances(c, "foo", "bar", 42), ShouldBeNil)
		i, err = module.NumInstances(c, "foo", "bar")
		So(i, ShouldEqual, 42)
		So(err, ShouldBeNil)

		i, err = module.NumInstances(c, "foo", "baz")
		So(i, ShouldEqual, 1)
		So(err, ShouldBeNil)
	})
}
