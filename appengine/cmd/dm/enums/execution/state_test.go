// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package execution

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
			s := Scheduled
			So(s.Evolve(Scheduled), ShouldBeNil)
			So(s, ShouldEqual, Scheduled)
		})

		Convey("Transition", func() {
			s := Running
			So(s.Evolve(Finished), ShouldBeNil)
			So(s, ShouldEqual, Finished)
		})

		Convey("Invalid starting transistion", func() {
			s := UnknownState
			So(s.Evolve(Scheduled), ShouldErrLike, "no transitions defined")
			So(s, ShouldEqual, UnknownState)
		})

		Convey("Invalid ending transistion", func() {
			s := Running
			So(s.Evolve(Scheduled), ShouldErrLike, "invalid state transition Running -> Scheduled")
			So(s, ShouldEqual, Running)
		})

		Convey("MustEvolve", func() {
			s := UnknownState
			So(func() { s.MustEvolve(Scheduled) }, ShouldPanic)
		})
	})
}
