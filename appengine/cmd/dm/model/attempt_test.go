// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"math"
	"testing"

	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/enums/attempt"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/bit_field"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAttempt(t *testing.T) {
	t.Parallel()

	Convey("Attempt", t, func() {
		Convey("ChangeState", func() {
			a := &Attempt{}
			So(a.State, ShouldEqual, attempt.UnknownState)
			So(a.State.Evolve(attempt.AddingDeps), ShouldErrLike, "invalid state transition")

			a.State = attempt.NeedsExecution
			So(a.State.Evolve(attempt.Executing), ShouldBeNil)
			So(a.State, ShouldEqual, attempt.Executing)

			So(a.State.Evolve(attempt.AddingDeps), ShouldBeNil)
			So(a.State.Evolve(attempt.Blocked), ShouldBeNil)
			So(a.State.Evolve(attempt.Blocked), ShouldBeNil)
			So(a.State.Evolve(attempt.NeedsExecution), ShouldBeNil)
			So(a.State.Evolve(attempt.Executing), ShouldBeNil)
			So(a.State.Evolve(attempt.Finished), ShouldBeNil)

			So(a.State.Evolve(attempt.NeedsExecution), ShouldErrLike, "invalid")
			So(a.State, ShouldEqual, attempt.Finished)
		})

		Convey("ToDisplay", func() {
			a := NewAttempt("quest", 10)
			a.State = attempt.Finished
			a.CurExecution = math.MaxUint32
			a.AddingDepsBitmap = bf.Make(20)
			a.WaitingDepBitmap = bf.Make(20)
			a.ResultExpiration = testclock.TestTimeUTC

			a.WaitingDepBitmap.Set(1)
			a.WaitingDepBitmap.Set(5)
			a.WaitingDepBitmap.Set(7)
			So(a.ToDisplay(), ShouldResemble, &display.Attempt{
				ID:             types.AttemptID{QuestID: "quest", AttemptNum: 10},
				NumExecutions:  math.MaxUint32,
				State:          attempt.Finished,
				Expiration:     testclock.TestTimeUTC,
				NumWaitingDeps: 17,
			})
		})
	})
}
