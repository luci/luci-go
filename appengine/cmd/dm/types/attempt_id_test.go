// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"sort"
	"testing"

	"github.com/luci/gae/service/datastore"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAttemptID(t *testing.T) {
	t.Parallel()

	Convey("Test AttemptID", t, func() {
		Convey("render", func() {
			aid := &AttemptID{}
			So(aid.ID(), ShouldEqual, "|ffffffff")

			aid.QuestID = "moo"
			So(aid.ID(), ShouldEqual, "moo|ffffffff")

			aid.AttemptNum = 10
			So(aid.ID(), ShouldEqual, "moo|fffffff5")

			p, err := aid.ToProperty()
			So(err, ShouldBeNil)
			So(p, ShouldResembleV, datastore.MkPropertyNI("moo|fffffff5"))
		})

		Convey("cmp", func() {
			aid1 := &AttemptID{}
			aid2 := &AttemptID{}
			So(aid1.Less(aid2), ShouldBeFalse)
			So(aid2.Less(aid1), ShouldBeFalse)

			aid1.QuestID = "a"
			So(aid1.Less(aid2), ShouldBeFalse)
			So(aid2.Less(aid1), ShouldBeTrue)

			aid2.AttemptNum++
			So(aid1.Less(aid2), ShouldBeFalse)
			So(aid2.Less(aid1), ShouldBeTrue)

			aid2.QuestID = "a"
			So(aid1.Less(aid2), ShouldBeTrue)
			So(aid2.Less(aid1), ShouldBeFalse)
		})

		Convey("slice sort", func() {
			rnd := AttemptIDSlice{
				{"b", 30},
				{"a", 2},
				{"b", 20},
				{"a", 10},
				{"c", 13},
				{"d", 0},
			}
			s := make(AttemptIDSlice, len(rnd))
			copy(s, rnd)

			sort.Sort(s)

			So(s, ShouldResembleV, AttemptIDSlice{
				{"a", 2},
				{"a", 10},
				{"b", 20},
				{"b", 30},
				{"c", 13},
				{"d", 0},
			})
		})

		Convey("parse", func() {
			Convey("good", func() {
				aid := NewAttemptID("something|ffffffff")
				So(aid, ShouldResembleV, &AttemptID{"something", 0})

				So(aid.FromProperty(datastore.MkPropertyNI("wat|fffffffa")), ShouldBeNil)
				So(aid, ShouldResembleV, &AttemptID{"wat", 5})
			})

			Convey("err", func() {
				aid := &AttemptID{}
				So(aid.SetEncoded("somethingfatsrnt"), ShouldErrLike, "unable to parse")
				So(aid.SetEncoded("something|cat"), ShouldErrLike, `strconv.ParseUint: parsing "cat"`)
				So(aid.SetEncoded(""), ShouldErrLike, "unable to parse")
				So(func() { NewAttemptID("somethingffffffff") }, ShouldPanic)
				So(aid.FromProperty(datastore.MkPropertyNI(100)), ShouldErrLike, "wrong type")
			})

		})
	})
}
