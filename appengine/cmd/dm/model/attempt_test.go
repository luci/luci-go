// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package model

import (
	"math"
	"testing"
	"time"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/luci/luci-go/common/bit_field"
	"github.com/luci/luci-go/common/clock/testclock"
	google_pb "github.com/luci/luci-go/common/proto/google"
	. "github.com/luci/luci-go/common/testing/assertions"

	"github.com/luci/luci-go/common/api/dm/service/v1"
)

func TestAttempt(t *testing.T) {
	t.Parallel()

	Convey("Attempt", t, func() {
		c := context.Background()
		c, clk := testclock.UseTime(c, testclock.TestTimeUTC)

		Convey("ModifyState", func() {
			a := MakeAttempt(c, dm.NewAttemptID("quest", 5))
			So(a.State, ShouldEqual, dm.Attempt_NEEDS_EXECUTION)
			So(a.ModifyState(c, dm.Attempt_ADDING_DEPS), ShouldErrLike, "invalid state transition")
			So(a.Modified, ShouldResemble, testclock.TestTimeUTC)

			clk.Add(time.Second)

			So(a.ModifyState(c, dm.Attempt_EXECUTING), ShouldBeNil)
			So(a.State, ShouldEqual, dm.Attempt_EXECUTING)
			So(a.Modified, ShouldResemble, clk.Now())

			So(a.ModifyState(c, dm.Attempt_ADDING_DEPS), ShouldBeNil)
			So(a.ModifyState(c, dm.Attempt_BLOCKED), ShouldBeNil)
			So(a.ModifyState(c, dm.Attempt_BLOCKED), ShouldBeNil)
			So(a.ModifyState(c, dm.Attempt_NEEDS_EXECUTION), ShouldBeNil)
			So(a.ModifyState(c, dm.Attempt_EXECUTING), ShouldBeNil)
			So(a.ModifyState(c, dm.Attempt_FINISHED), ShouldBeNil)

			So(a.ModifyState(c, dm.Attempt_NEEDS_EXECUTION), ShouldErrLike, "invalid")
			So(a.State, ShouldEqual, dm.Attempt_FINISHED)
		})

		Convey("ToProto", func() {
			Convey("NeedsExecution", func() {
				a := MakeAttempt(c, dm.NewAttemptID("quest", 10))

				So(a.ToProto(true), ShouldResemble, &dm.Attempt{
					Id: &dm.Attempt_ID{Quest: "quest", Id: 10},
					Data: &dm.Attempt_Data{
						Created:       google_pb.NewTimestamp(testclock.TestTimeUTC),
						Modified:      google_pb.NewTimestamp(testclock.TestTimeUTC),
						NumExecutions: 0,
						AttemptType: &dm.Attempt_Data_NeedsExecution_{NeedsExecution: &dm.Attempt_Data_NeedsExecution{
							Pending: google_pb.NewTimestamp(testclock.TestTimeUTC)}},
					},
				})
			})

			Convey("Executing", func() {
				a := MakeAttempt(c, dm.NewAttemptID("quest", 10))
				clk.Add(10 * time.Second)
				a.CurExecution = 1
				So(a.ModifyState(c, dm.Attempt_EXECUTING), ShouldBeNil)

				So(a.ToProto(true), ShouldResemble, &dm.Attempt{
					Id: &dm.Attempt_ID{Quest: "quest", Id: 10},
					Data: &dm.Attempt_Data{
						Created:       google_pb.NewTimestamp(testclock.TestTimeUTC),
						Modified:      google_pb.NewTimestamp(clk.Now()),
						NumExecutions: 1,
						AttemptType: &dm.Attempt_Data_Executing_{Executing: &dm.Attempt_Data_Executing{
							CurExecutionId: 1}}},
				})
			})

			Convey("AddingDeps", func() {
				a := MakeAttempt(c, dm.NewAttemptID("quest", 10))
				clk.Add(10 * time.Second)
				a.CurExecution = 1
				So(a.ModifyState(c, dm.Attempt_EXECUTING), ShouldBeNil)
				clk.Add(10 * time.Second)
				So(a.ModifyState(c, dm.Attempt_ADDING_DEPS), ShouldBeNil)
				a.AddingDepsBitmap = bf.Make(4)
				a.AddingDepsBitmap.Set(1)
				a.AddingDepsBitmap.Set(3)
				a.WaitingDepBitmap = bf.Make(4)

				So(a.ToProto(true), ShouldResemble, &dm.Attempt{
					Id: &dm.Attempt_ID{Quest: "quest", Id: 10},
					Data: &dm.Attempt_Data{
						Created:       google_pb.NewTimestamp(testclock.TestTimeUTC),
						Modified:      google_pb.NewTimestamp(clk.Now()),
						NumExecutions: 1,
						AttemptType: &dm.Attempt_Data_AddingDeps_{AddingDeps: &dm.Attempt_Data_AddingDeps{
							NumAdding:  2,
							NumWaiting: 4}}},
				})
			})

			Convey("Blocked", func() {
				a := MakeAttempt(c, dm.NewAttemptID("quest", 10))
				clk.Add(10 * time.Second)
				a.CurExecution = 1
				So(a.ModifyState(c, dm.Attempt_EXECUTING), ShouldBeNil)
				clk.Add(10 * time.Second)
				So(a.ModifyState(c, dm.Attempt_ADDING_DEPS), ShouldBeNil)
				a.WaitingDepBitmap = bf.Make(4)
				a.WaitingDepBitmap.Set(2)
				// don't increment the time: let the automatic microsecond advancement
				// take effect.
				So(a.ModifyState(c, dm.Attempt_BLOCKED), ShouldBeNil)

				So(a.ToProto(true), ShouldResemble, &dm.Attempt{
					Id: &dm.Attempt_ID{Quest: "quest", Id: 10},
					Data: &dm.Attempt_Data{
						Created:       google_pb.NewTimestamp(testclock.TestTimeUTC),
						Modified:      google_pb.NewTimestamp(clk.Now().Add(time.Microsecond)),
						NumExecutions: 1,
						AttemptType: &dm.Attempt_Data_Blocked_{Blocked: &dm.Attempt_Data_Blocked{
							NumWaiting: 3}}},
				})
			})

			Convey("Finished", func() {
				a := MakeAttempt(c, dm.NewAttemptID("quest", 10))
				a.State = dm.Attempt_FINISHED
				a.CurExecution = math.MaxUint32
				a.AddingDepsBitmap = bf.Make(20)
				a.WaitingDepBitmap = bf.Make(20)
				a.ResultExpiration = testclock.TestTimeUTC.Add(10 * time.Second)

				a.WaitingDepBitmap.Set(1)
				a.WaitingDepBitmap.Set(5)
				a.WaitingDepBitmap.Set(7)

				So(a.ToProto(true), ShouldResemble, &dm.Attempt{
					Id: &dm.Attempt_ID{Quest: "quest", Id: 10},
					Data: &dm.Attempt_Data{
						Created:       google_pb.NewTimestamp(testclock.TestTimeUTC),
						Modified:      google_pb.NewTimestamp(testclock.TestTimeUTC),
						NumExecutions: math.MaxUint32,
						AttemptType: &dm.Attempt_Data_Finished_{Finished: &dm.Attempt_Data_Finished{
							Expiration: google_pb.NewTimestamp(testclock.TestTimeUTC.Add(10 * time.Second))}},
					},
				})
			})
		})
	})
}
