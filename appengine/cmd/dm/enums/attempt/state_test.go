// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package attempt

import (
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestState(t *testing.T) {
	t.Parallel()

	Convey("State string", t, func() {
		So(UnknownState.String(), ShouldEqual, "UnknownState")
		So(State(127).String(), ShouldEqual, "State(127)")
	})

	Convey("Evolve", t, func() {
		Convey("Identity", func() {
			s := NeedsExecution
			So(s.Evolve(NeedsExecution), ShouldBeNil)
			So(s, ShouldEqual, NeedsExecution)
		})

		Convey("Transition", func() {
			s := Executing
			So(s.Evolve(AddingDeps), ShouldBeNil)
			So(s, ShouldEqual, AddingDeps)
		})

		Convey("Invalid starting transistion", func() {
			s := UnknownState
			So(s.Evolve(NeedsExecution), ShouldErrLike, "no transitions defined")
			So(s, ShouldEqual, UnknownState)
		})

		Convey("Invalid ending transistion", func() {
			s := Blocked
			So(s.Evolve(Finished), ShouldErrLike, "invalid state transition Blocked -> Finished")
			So(s, ShouldEqual, Blocked)
		})

		Convey("MustEvolve", func() {
			s := UnknownState
			So(func() { s.MustEvolve(NeedsExecution) }, ShouldPanic)
		})
	})
}
