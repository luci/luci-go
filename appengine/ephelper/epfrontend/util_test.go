// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package epfrontend

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSafeURLPathJoin(t *testing.T) {
	Convey(`Testing safeURLPathJoin`, t, func() {
		for _, e := range []struct {
			parts []string
			value string
		}{
			{[]string{"foo", "bar", "baz"}, "foo/bar/baz"},
			{[]string{"", "foo", "bar", "baz", ""}, "/foo/bar/baz/"},
			{[]string{"/", "/foo/", "/bar/", "//baz///"}, "/foo/bar/baz"},
			{[]string{""}, ""},
		} {
			Convey(fmt.Sprintf(`URL parts %v resolve to %q.`, e.parts, e.value), func() {
				So(safeURLPathJoin(e.parts...), ShouldEqual, e.value)
			})
		}
	})
}
