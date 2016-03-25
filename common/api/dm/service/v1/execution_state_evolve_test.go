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
			s := Execution_SCHEDULED
			So(s.Evolve(Execution_SCHEDULED), ShouldBeNil)
			So(s, ShouldEqual, Execution_SCHEDULED)
		})

		Convey("Transition", func() {
			s := Execution_RUNNING
			So(s.Evolve(Execution_FINISHED), ShouldBeNil)
			So(s, ShouldEqual, Execution_FINISHED)
		})

		Convey("Invalid starting transistion", func() {
			s := Execution_FINISHED
			So(s.Evolve(Execution_SCHEDULED), ShouldErrLike, "invalid state transition FINISHED -> SCHEDULE")
			So(s, ShouldEqual, Execution_FINISHED)
		})

		Convey("Invalid ending transistion", func() {
			s := Execution_RUNNING
			So(s.Evolve(Execution_SCHEDULED), ShouldErrLike, "invalid state transition RUNNING -> SCHEDULED")
			So(s, ShouldEqual, Execution_RUNNING)
		})

		Convey("MustEvolve", func() {
			s := Execution_FINISHED
			So(func() { s.MustEvolve(Execution_SCHEDULED) }, ShouldPanic)
		})
	})
}
