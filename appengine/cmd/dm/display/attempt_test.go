// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"
	"testing"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAttempt(t *testing.T) {
	t.Parallel()

	Convey("Attempt", t, func() {
		Convey("Less compares ids", func() {
			a1 := &Attempt{ID: *types.NewAttemptID("a|a")}
			a2 := &Attempt{ID: *types.NewAttemptID("b|a")}

			So(a1.Less(a2), ShouldBeTrue)
			So(a2.Less(a1), ShouldBeFalse)
		})

		Convey("slice", func() {
			r := newRand()
			s := make(AttemptSlice, 40)

			for i := range s {
				s[i] = &Attempt{ID: *randAttemptID(r)}
			}
			c := &Attempt{ID: *types.NewAttemptID("control|1")}
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
				So(s.Get(types.NewAttemptID("control|1")), ShouldEqual, c)
				So(s.Get(types.NewAttemptID("control|2")), ShouldBeNil)
			})

			Convey("can Merge into it", func() {
				inA := &Attempt{ID: *types.NewAttemptID("control|2")}
				So(s.Merge(inA), ShouldEqual, inA)
				So(s.Merge(inA), ShouldBeNil)
				So(s.Get(&inA.ID), ShouldEqual, inA)
				So(s.Get(types.NewAttemptID("control|1")), ShouldEqual, c)
			})

			Convey("Merging at the very end doesn't need a sort", func() {
				inA := &Attempt{ID: s[len(s)-1].ID}
				inA.ID.AttemptNum++
				So(s.Merge(inA), ShouldEqual, inA)
				So(sort.IsSorted(s), ShouldBeTrue)
			})

			Convey("Merging nil is nop", func() {
				So(s.Merge(nil), ShouldBeNil)
			})
		})
	})
}
