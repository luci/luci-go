// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestU32s(t *testing.T) {
	t.Parallel()

	Convey("U32s", t, func() {
		rnd := U32s{
			3, 1, 828, 1369, 29, 1451, 1392, 784, 3455, 1988, 2215, 41, 1811, 2926,
			1323, 2562, 1167, 592, 336, 1023, 1520, 2470,
		}
		ord := U32s{
			1, 3, 29, 41, 336, 592, 784, 828, 1023, 1167, 1323, 1369, 1392, 1451,
			1520, 1811, 1988, 2215, 2470, 2562, 2926, 3455,
		}

		Convey("sort", func() {
			sort.Sort(rnd)
			So(rnd, ShouldResemble, ord)

			Convey("has", func() {
				So(rnd.Has(592), ShouldBeTrue)
				So(rnd.Has(593), ShouldBeFalse)
				So(rnd.Has(594), ShouldBeFalse)
			})
		})
	})
}
