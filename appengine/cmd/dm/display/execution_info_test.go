// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"math/rand"
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func randExecutionInfoSlice(r *rand.Rand, n int) ExecutionInfoSlice {
	ret := make(ExecutionInfoSlice, n)
	for i := range ret {
		ret[i] = &ExecutionInfo{ExecutionID: r.Uint32()}
	}
	sort.Sort(ret)
	return ret
}

func TestExecutionInfo(t *testing.T) {
	t.Parallel()

	Convey("ExecutionInfo", t, func() {
		Convey("Less compares ids", func() {
			e1 := &ExecutionInfo{ExecutionID: 10}
			e2 := &ExecutionInfo{ExecutionID: 13}

			So(e1.Less(e2), ShouldBeTrue)
			So(e2.Less(e1), ShouldBeFalse)
		})

		Convey("slice", func() {
			s := randExecutionInfoSlice(newRand(), 40)
			c := s[0]

			Convey("sorts correctly", func() {
				for i := range s {
					if i == 0 {
						continue
					}
					So(s.Less(i-1, i), ShouldBeTrue)
				}
			})

			Convey("can Get from it", func() {
				So(s.Get(c.ExecutionID), ShouldEqual, c)
				So(s.Get(1337), ShouldBeNil)
			})

			Convey("can Merge into it", func() {
				in := &ExecutionInfo{ExecutionID: c.ExecutionID + 1}
				So(s.Merge(in), ShouldEqual, in)
				So(s.Merge(in), ShouldBeNil)
				So(s.Get(in.ExecutionID), ShouldEqual, in)
				So(s.Get(c.ExecutionID), ShouldEqual, c)
			})

			Convey("Merging at the very end doesn't need a sort", func() {
				in := &ExecutionInfo{ExecutionID: s[len(s)-1].ExecutionID + 1}
				So(s.Merge(in), ShouldEqual, in)
				So(sort.IsSorted(s), ShouldBeTrue)
			})

			Convey("Merging nil is nop", func() {
				So(s.Merge(nil), ShouldBeNil)
			})
		})
	})
}
