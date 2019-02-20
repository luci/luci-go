// Copyright 2019 The LUCI Authors.
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

package common

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFuncs(t *testing.T) {
	Convey("Time Tests", t, func() {

		Convey("Interval", func() {
			from := time.Date(2019, time.February, 3, 4, 5, 0, 0, time.UTC)
			to := time.Date(2019, time.February, 3, 4, 6, 0, 0, time.UTC)
			now := time.Date(2019, time.February, 3, 4, 7, 0, 0, time.UTC)
			Convey("Not started", func() {
				i := Interval{time.Time{}, time.Time{}, now}
				So(i.Started(), ShouldBeFalse)
				So(i.Ended(), ShouldBeFalse)
				So(i.Duration(), ShouldEqual, 0)
			})
			Convey("Started, not ended", func() {
				i := Interval{from, time.Time{}, now}
				So(i.Started(), ShouldBeTrue)
				So(i.Ended(), ShouldBeFalse)
				So(i.Duration(), ShouldEqual, 2*time.Minute)
			})
			Convey("Started and ended", func() {
				i := Interval{from, to, now}
				So(i.Started(), ShouldBeTrue)
				So(i.Ended(), ShouldBeTrue)
				So(i.Duration(), ShouldEqual, 1*time.Minute)
			})
			Convey("Ended before started", func() {
				i := Interval{to, from, now}
				So(i.Started(), ShouldBeTrue)
				So(i.Ended(), ShouldBeTrue)
				So(i.Duration(), ShouldEqual, 0)
			})
			Convey("Ended, not started", func() {
				i := Interval{time.Time{}, to, now}
				So(i.Started(), ShouldBeFalse)
				So(i.Ended(), ShouldBeTrue)
				So(i.Duration(), ShouldEqual, 0)
			})
		})

		Convey("humanDuration", func() {
			Convey("3 hrs", func() {
				h := HumanDuration(3 * time.Hour)
				So(h, ShouldEqual, "3 hrs")
			})

			Convey("2 hrs 59 mins", func() {
				h := HumanDuration(2*time.Hour + 59*time.Minute)
				So(h, ShouldEqual, "2 hrs 59 mins")
			})
		})
	})
}
