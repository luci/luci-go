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
	"context"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/data/rand/mathrand"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewBuildIDs(t *testing.T) {
	t.Parallel()

	Convey("NewBuildIDs", t, func() {
		ctx := mathrand.Set(context.Background(), rand.New(rand.NewSource(0)))
		ts := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
		So(1<<16, ShouldEqual, 65536)

		Convey("zero", func() {
			ids := NewBuildIDs(ctx, ts, 0)
			So(ids, ShouldBeEmpty)
		})

		Convey("one", func() {
			ids := NewBuildIDs(ctx, ts, 1)
			So(ids, ShouldResemble, []int64{
				0x7DB4463C7FF2FFA1,
			})
			So(ids[0]>>63, ShouldEqual, 0)
			So(ids[0]&0x0FFFFFFFFFFFFFFF>>buildIDTimeSuffixLen, ShouldEqual, 941745227775)
			So(ids[0]&0x0000000000000001, ShouldEqual, buildIDVersion)
		})

		Convey("two", func() {
			ids := NewBuildIDs(ctx, ts, 2)
			So(ids, ShouldResemble, []int64{
				0x7DB4463C7FFA8F71,
				0x7DB4463C7FFA8F61,
			})
			So(ids[0]>>63, ShouldEqual, 0)
			So(ids[0]&0x0000000000000001, ShouldEqual, buildIDVersion)
			So(ids[0]&0x0FFFFFFFFFFFFFFF>>buildIDTimeSuffixLen, ShouldEqual, 941745227775)
			So(ids[1]>>63, ShouldEqual, 0)
			So(ids[1]&0x0000000000000001, ShouldEqual, buildIDVersion)
			So(ids[1]&0x0FFFFFFFFFFFFFFF>>buildIDTimeSuffixLen, ShouldEqual, 941745227775)
		})

		Convey("many", func() {
			for i := 0; i < 2^16; i++ {
				ids := NewBuildIDs(ctx, ts, i)
				So(ids, ShouldHaveLength, i)
				prev := BuildIDMax
				for _, id := range ids {
					// Ensure strictly decreasing.
					So(id, ShouldBeLessThan, prev)
					prev = id
					// Ensure positive.
					So(id>>63, ShouldEqual, 0)
					// Ensure time component.
					So(id&0x0FFFFFFFFFFFFFFF>>buildIDTimeSuffixLen, ShouldEqual, 941745227775)
					// Ensure version.
					So(id&0x000000000000000F, ShouldEqual, buildIDVersion)
				}
			}
		})
	})
}

func TestIDRange(t *testing.T) {
	t.Parallel()

	Convey("IDRange", t, func() {
		Convey("valid time", func() {
			timeLow := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
			timeHigh := timeLow.Add(timeResolution * 10000)
			idLow, idHigh := IDRange(timeLow, timeHigh)

			inRange := func(t time.Time, suffix int64) bool {
				buildID := idTimeSegment(t) | suffix
				return idLow <= buildID && buildID < idHigh
			}
			ones := (int64(1) << buildIDTimeSuffixLen) - 1

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
			idLow, idHigh := IDRange(timeLow, timeHigh)
			So(idLow, ShouldEqual, 0)
			So(idHigh, ShouldEqual, 0)
		})
	})
}

func TestIDTimeSegment(t *testing.T) {
	t.Parallel()

	Convey("idTimeSegment", t, func() {
		Convey("after the start of the word time", func() {
			id := idTimeSegment(beginningOfTheWorld.Add(timeResolution))
			So(id, ShouldEqual, 0x7FFFFFFFFFE00000)
		})

		Convey("at the start of the word time", func() {
			id := idTimeSegment(beginningOfTheWorld)
			So(id, ShouldEqual, 0x7FFFFFFFFFF00000)
		})

		Convey("before the start of the word time", func() {
			ts := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			id := idTimeSegment(ts)
			So(id, ShouldEqual, 0)
		})
	})
}

func TestMayContainBuilds(t *testing.T) {
	t.Parallel()

	Convey("normal", t, func() {
		low := time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC)
		high := time.Date(2011, 2, 1, 0, 0, 0, 0, time.UTC)
		So(MayContainBuilds(low, high), ShouldBeTrue)
	})

	Convey("low time is larger than high time", t, func() {
		low := time.Date(2011, 2, 1, 0, 0, 0, 0, time.UTC)
		high := time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC)
		So(MayContainBuilds(low, high), ShouldBeFalse)
	})

	Convey("low and high time are nil", t, func() {
		low := time.Time{}
		high := time.Time{}
		So(MayContainBuilds(low, high), ShouldBeTrue)
	})

	Convey("high time is less than beginningOfTheWorld", t, func() {
		low := time.Date(2011, 2, 1, 0, 0, 0, 0, time.UTC)
		high := beginningOfTheWorld.Add(-1)
		So(MayContainBuilds(low, high), ShouldBeFalse)
	})
}
