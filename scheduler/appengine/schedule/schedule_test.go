// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schedule

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	epoch           = parseTime("2015-09-14 22:42:00 +0000 UTC")
	closestMidnight = parseTime("2015-09-15 00:00:00 +0000 UTC")
)

func parseTime(t string) time.Time {
	tm, err := time.Parse("2006-01-02 15:04:05.999 -0700 MST", t)
	if err != nil {
		panic(err)
	}
	return tm
}

func timeTable(cron string, now time.Time, count int) (out []time.Time) {
	s, err := Parse(cron, 0)
	if err != nil {
		panic(err)
	}
	prev := time.Time{}
	for ; count > 0; count-- {
		next := s.Next(now, prev)
		out = append(out, next)
		prev = next
		now = next.Add(time.Millisecond)
	}
	return
}

func TestAbsoluteSchedule(t *testing.T) {
	t.Parallel()

	Convey("Parsing success", t, func() {
		sched, err := Parse("* * * * * *", 0)
		So(err, ShouldBeNil)
		So(sched.String(), ShouldEqual, "* * * * * *")
	})

	Convey("Parsing error", t, func() {
		sched, err := Parse("not a schedule", 0)
		So(err, ShouldNotBeNil)
		So(sched, ShouldBeNil)
	})

	Convey("Next works", t, func() {
		sched, _ := Parse("*/15 * * * * * *", 0)
		So(sched.IsAbsolute(), ShouldBeTrue)
		So(sched.Next(epoch, time.Time{}), ShouldResemble, epoch.Add(15*time.Second))
		So(sched.Next(epoch.Add(15*time.Second), epoch), ShouldResemble, epoch.Add(30*time.Second))
	})

	Convey("Each 3 hours time table", t, func() {
		So(timeTable("0 */3 * * * *", epoch, 4), ShouldResemble, []time.Time{
			closestMidnight,
			closestMidnight.Add(3 * time.Hour),
			closestMidnight.Add(6 * time.Hour),
			closestMidnight.Add(9 * time.Hour),
		})
		So(timeTable("0 1/3 * * * *", epoch, 4), ShouldResemble, []time.Time{
			closestMidnight.Add(1 * time.Hour),
			closestMidnight.Add(4 * time.Hour),
			closestMidnight.Add(7 * time.Hour),
			closestMidnight.Add(10 * time.Hour),
		})
	})

	Convey("Trailing stars are optional", t, func() {
		// Exact same time tables.
		So(timeTable("0 */3 * * *", epoch, 4), ShouldResemble, timeTable("0 */3 * * * *", epoch, 4))
	})

	Convey("List of hours", t, func() {
		So(timeTable("0 2,10,18 * * *", epoch, 4), ShouldResemble, []time.Time{
			closestMidnight.Add(2 * time.Hour),
			closestMidnight.Add(10 * time.Hour),
			closestMidnight.Add(18 * time.Hour),
			closestMidnight.Add(26 * time.Hour),
		})
	})

	Convey("Once a day", t, func() {
		So(timeTable("0 7 * * *", epoch, 4), ShouldResemble, []time.Time{
			closestMidnight.Add(7 * time.Hour),
			closestMidnight.Add(31 * time.Hour),
			closestMidnight.Add(55 * time.Hour),
			closestMidnight.Add(79 * time.Hour),
		})
	})
}

func TestRelativeSchedule(t *testing.T) {
	t.Parallel()

	Convey("Parsing success", t, func() {
		sched, err := Parse("with 15s interval", 0)
		So(err, ShouldBeNil)
		So(sched.String(), ShouldEqual, "with 15s interval")
	})

	Convey("Parsing error", t, func() {
		sched, err := Parse("with bladasdafier", 0)
		So(err, ShouldNotBeNil)
		So(sched, ShouldBeNil)

		sched, err = Parse("with -1s interval", 0)
		So(err, ShouldNotBeNil)
		So(sched, ShouldBeNil)

		sched, err = Parse("with NaNs interval", 0)
		So(err, ShouldNotBeNil)
		So(sched, ShouldBeNil)
	})

	Convey("Next works", t, func() {
		sched, _ := Parse("with 15s interval", 0)
		So(sched.IsAbsolute(), ShouldBeFalse)

		// First tick is pseudorandom.
		So(sched.Next(epoch, time.Time{}), ShouldResemble, epoch.Add(14*time.Second+177942239*time.Nanosecond))

		// Next tick is 15s from prev one, or now if it's too late.
		So(sched.Next(epoch.Add(16*time.Second), epoch.Add(15*time.Second)), ShouldResemble, epoch.Add(30*time.Second))
		So(sched.Next(epoch.Add(31*time.Second), epoch.Add(15*time.Second)), ShouldResemble, epoch.Add(31*time.Second))
	})
}
