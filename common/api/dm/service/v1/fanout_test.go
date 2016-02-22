// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAttemptFanoutNormalize(t *testing.T) {
	t.Parallel()

	Convey("AttemptFanout.Normalize", t, func() {
		Convey("empty", func() {
			a := &AttemptFanout{}
			So(a.Normalize(), ShouldBeNil)
		})

		Convey("stuff", func() {
			a := &AttemptFanout{To: map[string]*AttemptFanout_AttemptNums{}}
			a.To["quest"] = &AttemptFanout_AttemptNums{Nums: []uint32{700, 2, 7, 90, 1, 1, 700}}
			a.To["other"] = &AttemptFanout_AttemptNums{}
			a.To["other2"] = &AttemptFanout_AttemptNums{Nums: []uint32{}}
			So(a.Normalize(), ShouldBeNil)

			So(a, ShouldResemble, &AttemptFanout{To: map[string]*AttemptFanout_AttemptNums{
				"quest":  {Nums: []uint32{700, 90, 7, 2, 1}},
				"other":  {},
				"other2": {},
			}})
		})

		Convey("empty attempts / 0 attempts", func() {
			a := &AttemptFanout{To: map[string]*AttemptFanout_AttemptNums{}}
			a.To["quest"] = nil
			a.To["other"] = &AttemptFanout_AttemptNums{Nums: []uint32{0}}
			a.To["woot"] = &AttemptFanout_AttemptNums{Nums: []uint32{}}
			So(a.Normalize(), ShouldBeNil)
			So(a, ShouldResemble, NewAttemptFanout(map[string][]uint32{
				"quest": nil,
				"other": nil,
				"woot":  nil,
			}))

			Convey("0 + other nums is error", func() {
				a.To["nerp"] = &AttemptFanout_AttemptNums{Nums: []uint32{30, 7, 0}}
				So(a.Normalize(), ShouldErrLike, "contains 0 as well as other values")
			})
		})
	})

	Convey("AttemptFanout.AddAIDs", t, func() {
		fanout := &AttemptFanout{}
		fanout.AddAIDs(NewAttemptID("a", 1), NewAttemptID("b", 1), NewAttemptID("b", 2))
		So(fanout.To, ShouldResemble, map[string]*AttemptFanout_AttemptNums{
			"a": {[]uint32{1}},
			"b": {[]uint32{1, 2}},
		})
	})
}
