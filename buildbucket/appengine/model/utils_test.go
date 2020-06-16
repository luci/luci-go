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

package model

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestModelUtils(t *testing.T) {
	t.Parallel()

	Convey("ModelUtils", t, func() {
		Convey("BuildIDRange", func() {
			Convey("valid time", func() {
				timeLow := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
				timeHigh := timeLow.Add(timeResolution * 10000)
				idLow, idHigh := BuildIDRange(timeLow, timeHigh)
				inRange := func(t time.Time, suffix int64) bool {
					buildID := idTimeSegment(t) | suffix
					return idLow <= buildID && buildID < idHigh
				}
				unit := timeResolution
				ones := int64((1 << buildIDSuffixLen) - 1)
				for _, suffix := range []int64{0, ones} {
					So(inRange(timeLow.Add(-unit), suffix), ShouldBeFalse)
					So(inRange(timeLow, suffix), ShouldBeTrue)
					So(inRange(timeLow.Add(unit), suffix), ShouldBeTrue)

					So(inRange(timeHigh.Add(-unit), suffix), ShouldBeTrue)
					So(inRange(timeHigh, suffix), ShouldBeFalse)
					So(inRange(timeHigh.Add(unit), suffix), ShouldBeFalse)
				}
			})

			Convey("invalid time", func() {
				timeLow := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
				timeHigh := timeLow.Add(timeResolution * 10000)
				idLow, idHigh := BuildIDRange(timeLow, timeHigh)
				So(idLow, ShouldEqual, 0)
				So(idHigh, ShouldEqual, 0)
			})
		})
	})
}
