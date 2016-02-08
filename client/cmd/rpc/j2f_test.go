// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEscape(t *testing.T) {
	t.Parallel()

	Convey("escape", t, func() {
		test := func(s, expected string) {
			Convey(s, func() {
				So(escapeFlag(s), ShouldEqual, expected)
			})
		}

		test("a", `a`)
		test("a b", `'a b'`)
		test("a\nb", "a\\\nb")
		test("a'b", `a\'b`)
		test("a' b", `a\'\ b`)
	})
}
