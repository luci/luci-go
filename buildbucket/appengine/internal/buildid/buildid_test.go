// Copyright 2020 The LUCI Authors.
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

package buildid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIdRange(t *testing.T) {
	t.Parallel()

	Convey("IdRange", t, func() {
		Convey("valid time", func() {
			timeLow := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
			timeHigh := timeLow.Add(timeResolution * 10000)
			idLow, idHigh := IdRange(timeLow, timeHigh)

			inRange := func(t time.Time, suffix int64) bool {
				buildID := idTimeSegment(t) | suffix
				return idLow <= buildID && buildID < idHigh
			}
			ones := (int64(1) << buildIdSuffixLen) - 1

			// Ensure that min and max possible build IDs are within
			// the range up to the timeResolution.
			for _, suffix := range []int64{0, ones} {
				So(inRange(timeLow.Add(-timeResolution), suffix), ShouldBeFalse)
				So(inRange(timeLow, suffix), ShouldBeTrue)
				So(inRange(timeLow.Add(timeResolution), suffix), ShouldBeTrue)

				So(inRange(timeHigh.Add(-timeResolution), suffix), ShouldBeTrue)
				So(inRange(timeHigh, suffix), ShouldBeFalse)
				So(inRange(timeHigh.Add(timeResolution), suffix), ShouldBeFalse)
			}
		})

		Convey("invalid time", func() {
			timeLow := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			timeHigh := timeLow.Add(timeResolution * 10000)
			idLow, idHigh := IdRange(timeLow, timeHigh)
			So(idLow, ShouldEqual, 0)
			So(idHigh, ShouldEqual, 0)
		})
	})
}

func TestIdTimeSegment(t *testing.T) {
	t.Parallel()

	Convey("beginningOfTheWorld is 2010-01-01", t, func() {
		t := time.Date(2010, 1, 1, 0, 0, 0, 1000000, time.UTC)
		So(beginningOfTheWorld, ShouldEqual, t.Unix())
	})

	Convey("idTimeSegment", t, func() {
		Convey("after the start of the word time", func() {
			id := idTimeSegment(time.Unix(beginningOfTheWorld, 0).Add(timeResolution))
			So(id, ShouldEqual, 0x7FFFFFFFFFE00000)
		})

		Convey("at the start of the word time", func() {
			id := idTimeSegment(time.Unix(beginningOfTheWorld, 0))
			So(id, ShouldEqual, 0x7FFFFFFFFFF00000)
		})

		Convey("before the start of the word time", func() {
			t := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			id := idTimeSegment(t)
			So(id, ShouldEqual, 0)
		})
	})
}
