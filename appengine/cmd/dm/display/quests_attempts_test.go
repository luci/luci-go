// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func randQuestsAttemptsSlice(r *rand.Rand, n int) QuestAttemptsSlice {
	s := make(QuestAttemptsSlice, n)
	for i := range s {
		s[i] = &QuestAttempts{randQuestID(r), randU32s(r, 20)}
	}
	return s
}

func TestQuestAttempts(t *testing.T) {
	t.Parallel()

	Convey("QuestAttempts", t, func() {
		Convey("Less compares quest ids", func() {
			So((&QuestAttempts{QuestID: "A"}).Less(&QuestAttempts{QuestID: "B"}),
				ShouldBeTrue)
		})

		Convey("slice", func() {
			s := randQuestsAttemptsSlice(newRand(), 40)

			c := &QuestAttempts{"control", types.U32s{100, 200, 300}}
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
				So(s.Get("bontrolz"), ShouldBeNil)
			})

			Convey("can Merge into it", func() {
				in := &QuestAttempts{"control", types.U32s{200, 291}}
				So(s.Merge(in), ShouldResembleV,
					&QuestAttempts{"control", types.U32s{291}})
				So(s.Merge(in), ShouldBeNil)
				So(s.Get("control").Attempts, ShouldResembleV, types.U32s{
					100, 200, 291, 300,
				})

				in = &QuestAttempts{"controlNew", types.U32s{20}}
				So(s.Merge(in), ShouldResembleV, in)
				So(sort.IsSorted(s), ShouldBeTrue)
			})

			Convey("merging at the very end doesn't need a sort", func() {
				in := &QuestAttempts{QuestID: s[len(s)-1].QuestID + "z"}
				So(s.Merge(in), ShouldResembleV, in)
				So(sort.IsSorted(s), ShouldBeTrue)
			})

		})
	})
}
