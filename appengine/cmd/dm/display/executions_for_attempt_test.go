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

func TestExecutionsForAttempt(t *testing.T) {
	t.Parallel()

	Convey("ExecutionsForAttempt", t, func() {
		Convey("Less compares originating attempt ids", func() {
			e1 := &ExecutionsForAttempt{Attempt: *types.NewAttemptID("a|a")}
			e2 := &ExecutionsForAttempt{Attempt: *types.NewAttemptID("b|a")}

			So(e1.Less(e2), ShouldBeTrue)
			So(e2.Less(e1), ShouldBeFalse)
		})

		Convey("slice", func() {
			r := newRand()
			s := make(ExecutionsForAttemptSlice, 40)
			for i := range s {
				s[i] = &ExecutionsForAttempt{
					*randAttemptID(r),
					randExecutionInfoSlice(r, 20),
				}
			}

			c := &ExecutionsForAttempt{
				*types.NewAttemptID("control|1"),
				ExecutionInfoSlice{
					{17, "someToken"},
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

			Convey("can Get Attempt it", func() {
				So(s.Get(types.NewAttemptID("control|1")), ShouldResemble, c)
				So(s.Get(types.NewAttemptID("nopwat|1")), ShouldBeNil)
			})

			Convey("Can merge into it", func() {
				in := &ExecutionsForAttempt{
					*types.NewAttemptID("control|1"),
					ExecutionInfoSlice{
						{13, "otherToken"},
						{17, ""},
					},
				}
				So(s.Merge(in), ShouldResemble, &ExecutionsForAttempt{
					*types.NewAttemptID("control|1"),
					ExecutionInfoSlice{
						{13, "otherToken"},
					},
				})
				So(s.Merge(in), ShouldBeNil)

				in = &ExecutionsForAttempt{
					*types.NewAttemptID("control|2"),
					ExecutionInfoSlice{
						{13, "otherToken"},
					},
				}
				So(s.Merge(in), ShouldResemble, in)
				So(s.Merge(in), ShouldBeNil)
			})

			Convey("Merging at the very end doesn't need a sort", func() {
				in := &ExecutionsForAttempt{
					s[len(s)-1].Attempt,
					ExecutionInfoSlice{
						{1, "other"},
					},
				}
				in.Attempt.AttemptNum++
				So(s.Merge(in), ShouldEqual, in)
				So(sort.IsSorted(s), ShouldBeTrue)
			})

			Convey("Merging nil is nop", func() {
				So(s.Merge(nil), ShouldBeNil)
			})
		})

	})
}
