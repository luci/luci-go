// Copyright 2024 The LUCI Authors.
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

package dsutils

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestHexPrefixRestrictionTracker(t *testing.T) {
	t.Parallel()

	Convey("HexPrefixRestrictionTracker", t, func() {
		Convey("TryClaim", func() {
			Convey("claim items", func() {
				rt := newHexPrefixRestrictionTracker(hexPrefixRestriction{
					HexPrefixLength: 6,
					Start:           "000000",
					End:             "ffffff",
				})
				So(rt.TryClaim(HexPosClaim{Value: "100000"}), ShouldBeTrue)
				So(rt.TryClaim(HexPosClaim{Value: "200000"}), ShouldBeTrue)
				So(rt.TryClaim(HexPosClaim{Value: "300000"}), ShouldBeTrue)
			})

			Convey("claim an item before start", func() {
				rt := newHexPrefixRestrictionTracker(hexPrefixRestriction{
					HexPrefixLength: 6,
					Start:           "bbbbbb",
					End:             "ffffff",
				})
				So(rt.TryClaim(HexPosClaim{Value: "aaabcd"}), ShouldBeFalse)
				So(rt.GetError(), ShouldErrLike, "out of bounds of the restriction")
			})

			Convey("claim an item repeatedly", func() {
				rt := newHexPrefixRestrictionTracker(hexPrefixRestriction{
					HexPrefixLength: 6,
					Start:           "000000",
					End:             "ffffff",
				})
				So(rt.TryClaim(HexPosClaim{Value: "aaabcd"}), ShouldBeTrue)

				So(rt.TryClaim(HexPosClaim{Value: "aaabcd"}), ShouldBeFalse)
				So(rt.GetError(), ShouldErrLike, "cannot claim a key", "smaller than the previously claimed key")
			})

			Convey("claim a smaller item", func() {
				rt := newHexPrefixRestrictionTracker(hexPrefixRestriction{
					HexPrefixLength: 6,
					Start:           "000000",
					End:             "ffffff",
				})
				So(rt.TryClaim(HexPosClaim{Value: "aaabcd"}), ShouldBeTrue)

				So(rt.TryClaim(HexPosClaim{Value: "111111"}), ShouldBeFalse)
				So(rt.GetError(), ShouldErrLike, "cannot claim a key", "smaller than the previously claimed key")
			})

			Convey("claim items after end", func() {
				rt := newHexPrefixRestrictionTracker(hexPrefixRestriction{
					HexPrefixLength: 6,
					Start:           "000000",
					End:             "aaaaaa",
				})

				// First claim should mark the restriction as completed.
				So(rt.TryClaim(HexPosClaim{Value: "aaabcd"}), ShouldBeFalse)
				So(rt.GetError(), ShouldBeNil)
				So(rt.IsDone(), ShouldBeTrue)

				// Second claim should trigger an error.
				So(rt.TryClaim(HexPosClaim{Value: "cccccc"}), ShouldBeFalse)
				So(rt.GetError(), ShouldErrLike, "cannot claim", "after the everything has been claimed")
			})
		})

		Convey("TrySplit", func() {
			Convey("with regular start and end", func() {
				rt := newHexPrefixRestrictionTracker(hexPrefixRestriction{
					HexPrefixLength:  6,
					StartIsExclusive: true,
					Start:            "00000",
					EndIsUnbounded:   false,
					EndIsExclusive:   true,
					End:              "fffff",
				})

				primary, residual, err := rt.TrySplit(0.5)
				So(err, ShouldBeNil)
				So(primary, ShouldResemble, hexPrefixRestriction{
					HexPrefixLength:  6,
					StartIsExclusive: true,
					Start:            "00000",
					EndIsUnbounded:   false,
					EndIsExclusive:   false,
					End:              "7ffff8",
				})
				So(residual, ShouldResemble, hexPrefixRestriction{
					HexPrefixLength:  6,
					StartIsExclusive: true,
					Start:            "7ffff8",
					EndIsUnbounded:   false,
					EndIsExclusive:   true,
					End:              "fffff",
				})
			})

			Convey("with empty start and end", func() {
				rt := newHexPrefixRestrictionTracker(hexPrefixRestriction{
					HexPrefixLength: 6,
					EndIsUnbounded:  true,
				})

				primary, residual, err := rt.TrySplit(0.5)
				So(err, ShouldBeNil)
				So(primary, ShouldResemble, hexPrefixRestriction{
					HexPrefixLength: 6,
					EndIsUnbounded:  false,
					EndIsExclusive:  false,
					End:             "7fffff",
				})
				So(residual, ShouldResemble, hexPrefixRestriction{
					HexPrefixLength:  6,
					StartIsExclusive: true,
					Start:            "7fffff",
					EndIsUnbounded:   true,
				})
			})

			Convey("when some items are claimed", func() {
				rt := newHexPrefixRestrictionTracker(hexPrefixRestriction{
					HexPrefixLength: 6,
					EndIsUnbounded:  true,
				})
				So(rt.TryClaim(HexPosClaim{Value: "aaabcd"}), ShouldBeTrue)

				primary, residual, err := rt.TrySplit(0.5)
				So(err, ShouldBeNil)
				So(primary, ShouldResemble, hexPrefixRestriction{
					HexPrefixLength: 6,
					EndIsUnbounded:  false,
					EndIsExclusive:  false,
					End:             "d555e6",
				})
				So(residual, ShouldResemble, hexPrefixRestriction{
					HexPrefixLength:  6,
					StartIsExclusive: true,
					Start:            "d555e6",
					EndIsUnbounded:   true,
				})
			})

			Convey("when self check-pointing", func() {
				rt := newHexPrefixRestrictionTracker(hexPrefixRestriction{
					HexPrefixLength: 6,
					EndIsUnbounded:  true,
				})
				So(rt.TryClaim(HexPosClaim{Value: "aaabcd"}), ShouldBeTrue)

				primary, residual, err := rt.TrySplit(0)
				So(err, ShouldBeNil)
				So(primary, ShouldResemble, hexPrefixRestriction{
					HexPrefixLength: 6,
					EndIsUnbounded:  false,
					EndIsExclusive:  false,
					End:             "aaabcd",
				})
				So(residual, ShouldResemble, hexPrefixRestriction{
					HexPrefixLength:  6,
					StartIsExclusive: true,
					Start:            "aaabcd",
					EndIsUnbounded:   true,
				})
			})
		})

	})
}
