// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"sort"
	"testing"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDepsFromAttempt(t *testing.T) {
	t.Parallel()

	Convey("DepsFromAttempt", t, func() {
		Convey("Less compares originating attempt ids", func() {
			d1 := &DepsFromAttempt{From: *types.NewAttemptID("a|a")}
			a2 := &DepsFromAttempt{From: *types.NewAttemptID("b|a")}

			So(d1.Less(a2), ShouldBeTrue)
			So(a2.Less(d1), ShouldBeFalse)
		})

		Convey("slice", func() {
			r := newRand()
			s := make(DepsFromAttemptSlice, 40)
			for i := range s {
				s[i] = &DepsFromAttempt{
					*randAttemptID(r),
					randQuestsAttemptsSlice(r, 20),
				}
				sort.Sort(s[i].To)
			}

			c := &DepsFromAttempt{
				*types.NewAttemptID("control|1"),
				QuestAttemptsSlice{
					{"other", types.U32s{1, 2, 3}},
				},
			}
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
				So(s.Get(types.NewAttemptID("control|1")), ShouldResembleV, c)
				So(s.Get(types.NewAttemptID("nopwat|1")), ShouldBeNil)
			})

			Convey("Can merge into it", func() {
				in := &DepsFromAttempt{
					*types.NewAttemptID("control|1"),
					QuestAttemptsSlice{
						{"other", types.U32s{3, 9}},
						{"other2", types.U32s{1, 2}},
					},
				}
				So(s.Merge(in), ShouldResembleV, &DepsFromAttempt{
					*types.NewAttemptID("control|1"),
					QuestAttemptsSlice{
						{"other", types.U32s{9}},
						{"other2", types.U32s{1, 2}},
					},
				})
				So(s.Merge(in), ShouldBeNil)

				in = &DepsFromAttempt{
					*types.NewAttemptID("control|2"),
					QuestAttemptsSlice{
						{"other", types.U32s{9}},
					},
				}
				So(s.Merge(in), ShouldResembleV, in)
				So(s.Merge(in), ShouldBeNil)
			})

			Convey("Merging at the very end doesn't need a sort", func() {
				in := &DepsFromAttempt{
					s[len(s)-1].From,
					QuestAttemptsSlice{
						{"other", types.U32s{3, 9}},
					},
				}
				in.From.AttemptNum++
				So(s.Merge(in), ShouldEqual, in)
				So(sort.IsSorted(s), ShouldBeTrue)
			})

			Convey("Merging nil is nop", func() {
				So(s.Merge(nil), ShouldBeNil)
			})
		})

	})
}
