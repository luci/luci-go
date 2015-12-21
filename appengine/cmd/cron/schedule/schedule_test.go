// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package schedule

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScheduleParsing(t *testing.T) {
	Convey("Parsing success", t, func() {
		sched, err := Parse("* * * * * *")
		So(err, ShouldBeNil)
		So(sched.String(), ShouldEqual, "* * * * * *")

		// Test cache hit.
		sched2, err := Parse("* * * * * *")
		So(err, ShouldBeNil)
		So(sched2, ShouldEqual, sched) // exact same pointer
	})

	Convey("Parsing error", t, func() {
		sched, err := Parse("not a schedule")
		So(err, ShouldNotBeNil)
		So(sched, ShouldBeNil)

		// Test cache hit.
		sched, err2 := Parse("not a schedule")
		So(sched, ShouldBeNil)
		So(err2, ShouldEqual, err) // exact same pointer
	})
}
