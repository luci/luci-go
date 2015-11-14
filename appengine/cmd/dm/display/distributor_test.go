// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func TestDistributor(t *testing.T) {
	t.Parallel()

	Convey("Distributor", t, func() {
		Convey("Less compares ids", func() {
			d1 := &Distributor{Name: "other"}
			d2 := &Distributor{Name: "swarming"}

			So(d1.Less(d2), ShouldBeTrue)
			So(d2.Less(d1), ShouldBeFalse)
		})

		Convey("slice", func() {
			r := newRand()
			s := make(DistributorSlice, 40)

			for i := range s {
				s[i] = &Distributor{Name: string(randByteSequence(r, 20, alpha))}
			}
			c := &Distributor{Name: "control"}
			s = append(s, c)
			sort.Sort(s)

			Convey("sorts correctly", func() {
				for i := range s {
					if i == 0 {
						continue
					}
					So(s.Less(i-1, i), ShouldBeTrue)
				}
			})

			Convey("can Get from it", func() {
				So(s.Get("control"), ShouldEqual, c)
				So(s.Get("watlol"), ShouldBeNil)
			})

			Convey("can Merge into it", func() {
				in := &Distributor{Name: "control2"}
				So(s.Merge(in), ShouldEqual, in)
				So(s.Merge(in), ShouldBeNil)
				So(s.Get(in.Name), ShouldEqual, in)
				So(s.Get("control"), ShouldEqual, c)
			})

			Convey("Merging at the very end doesn't need a sort", func() {
				in := &Distributor{Name: s[len(s)-1].Name + "z"}
				So(s.Merge(in), ShouldEqual, in)
				So(sort.IsSorted(s), ShouldBeTrue)
			})

			Convey("Merging nil is nop", func() {
				So(s.Merge(nil), ShouldBeNil)
			})
		})
	})
}
