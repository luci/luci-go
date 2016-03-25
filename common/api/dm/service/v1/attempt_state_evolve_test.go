// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAttemptState(t *testing.T) {
	t.Parallel()

	Convey("Evolve", t, func() {
		Convey("Identity", func() {
			s := Attempt_NEEDS_EXECUTION
			So(s.Evolve(Attempt_NEEDS_EXECUTION), ShouldBeNil)
			So(s, ShouldEqual, Attempt_NEEDS_EXECUTION)
		})

		Convey("Transition", func() {
			s := Attempt_EXECUTING
			So(s.Evolve(Attempt_ADDING_DEPS), ShouldBeNil)
			So(s, ShouldEqual, Attempt_ADDING_DEPS)
		})

		Convey("Invalid starting transistion", func() {
			s := Attempt_NEEDS_EXECUTION
			So(s.Evolve(Attempt_FINISHED), ShouldErrLike, "invalid state transition NEEDS_EXECUTION -> FINISHED")
			So(s, ShouldEqual, Attempt_NEEDS_EXECUTION)
		})

		Convey("Invalid ending transistion", func() {
			s := Attempt_BLOCKED
			So(s.Evolve(Attempt_FINISHED), ShouldErrLike, "invalid state transition BLOCKED -> FINISHED")
			So(s, ShouldEqual, Attempt_BLOCKED)
		})

		Convey("MustEvolve", func() {
			s := Attempt_FINISHED
			So(func() { s.MustEvolve(Attempt_NEEDS_EXECUTION) }, ShouldPanic)
		})
	})
}
