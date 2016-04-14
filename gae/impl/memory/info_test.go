// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"testing"

	"github.com/luci/gae/service/info"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMustNamespace(t *testing.T) {
	Convey("MustNamespace panics", t, func() {
		c := Use(context.Background())
		i := info.Get(c)

		So(func() {
			i.MustNamespace("valid_namespace_name")
		}, ShouldNotPanic)
		So(func() {
			i.MustNamespace("invalid namespace name")
		}, ShouldPanic)
	})

	Convey("Testable interface works", t, func() {
		c := Use(context.Background())
		c = useGID(c, func(mod *globalInfoData) {
			mod.appid = "app-id"
		})

		// Default value.
		So(info.Get(c).AppID(), ShouldEqual, "app-id")
		So(info.Get(c).RequestID(), ShouldEqual, "test-request-id")

		// Setting to "override" applies to initial context.
		c = info.Get(c).Testable().SetRequestID("override")
		So(info.Get(c).RequestID(), ShouldEqual, "override")

		// Derive inner context, "override" applies.
		c = info.Get(c).MustNamespace("valid_namespace_name")
		So(info.Get(c).RequestID(), ShouldEqual, "override")
	})
}
