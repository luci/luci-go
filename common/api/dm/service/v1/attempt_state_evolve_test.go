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
			s := Attempt_NeedsExecution
			So(s.Evolve(Attempt_NeedsExecution), ShouldBeNil)
			So(s, ShouldEqual, Attempt_NeedsExecution)
		})

		Convey("Transition", func() {
			s := Attempt_Executing
			So(s.Evolve(Attempt_AddingDeps), ShouldBeNil)
			So(s, ShouldEqual, Attempt_AddingDeps)
		})

		Convey("Invalid starting transistion", func() {
			s := Attempt_Unknown
			So(s.Evolve(Attempt_NeedsExecution), ShouldErrLike, "no transitions defined")
			So(s, ShouldEqual, Attempt_Unknown)
		})

		Convey("Invalid ending transistion", func() {
			s := Attempt_Blocked
			So(s.Evolve(Attempt_Finished), ShouldErrLike, "invalid state transition Blocked -> Finished")
			So(s, ShouldEqual, Attempt_Blocked)
		})

		Convey("MustEvolve", func() {
			s := Attempt_Unknown
			So(func() { s.MustEvolve(Attempt_NeedsExecution) }, ShouldPanic)
		})
	})
}
