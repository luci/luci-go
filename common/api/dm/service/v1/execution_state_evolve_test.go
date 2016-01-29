// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExecutionState(t *testing.T) {
	t.Parallel()

	Convey("Evolve", t, func() {
		Convey("Identity", func() {
			s := Execution_Scheduled
			So(s.Evolve(Execution_Scheduled), ShouldBeNil)
			So(s, ShouldEqual, Execution_Scheduled)
		})

		Convey("Transition", func() {
			s := Execution_Running
			So(s.Evolve(Execution_Finished), ShouldBeNil)
			So(s, ShouldEqual, Execution_Finished)
		})

		Convey("Invalid starting transistion", func() {
			s := Execution_Unknown
			So(s.Evolve(Execution_Scheduled), ShouldErrLike, "no transitions defined")
			So(s, ShouldEqual, Execution_Unknown)
		})

		Convey("Invalid ending transistion", func() {
			s := Execution_Running
			So(s.Evolve(Execution_Scheduled), ShouldErrLike, "invalid state transition Running -> Scheduled")
			So(s, ShouldEqual, Execution_Running)
		})

		Convey("MustEvolve", func() {
			s := Execution_Unknown
			So(func() { s.MustEvolve(Execution_Scheduled) }, ShouldPanic)
		})
	})
}
