// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQuest(t *testing.T) {
	t.Parallel()

	Convey("Quest", t, func() {
		Convey("Less compares ids", func() {
			q1 := &Quest{ID: "other"}
			q2 := &Quest{ID: "swarming"}

			So(q1.Less(q2), ShouldBeTrue)
			So(q2.Less(q1), ShouldBeFalse)
		})

		Convey("slice", func() {
			r := newRand()
			s := make(QuestSlice, 40)

			for i := range s {
				s[i] = &Quest{ID: string(randByteSequence(r, 20, alpha))}
			}
			c := &Quest{ID: "control"}
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
				in := &Quest{ID: "control2"}
				So(s.Merge(in), ShouldEqual, in)
				So(s.Merge(in), ShouldBeNil)
				So(s.Get(in.ID), ShouldEqual, in)
				So(s.Get("control"), ShouldEqual, c)
			})

			Convey("Merging at the very end doesn't need a sort", func() {
				in := &Quest{ID: s[len(s)-1].ID + "z"}
				So(s.Merge(in), ShouldEqual, in)
				So(sort.IsSorted(s), ShouldBeTrue)
			})

			Convey("Merging nil is nop", func() {
				So(s.Merge(nil), ShouldBeNil)
			})
		})
	})
}
