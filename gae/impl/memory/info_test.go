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
}
