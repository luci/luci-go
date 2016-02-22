// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"testing"

	"github.com/luci/gae/service/datastore"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAttemptID(t *testing.T) {
	t.Parallel()

	Convey("Test Attempt_ID DM encoding", t, func() {
		Convey("render", func() {
			aid := NewAttemptID("", 0)
			So(aid.DMEncoded(), ShouldEqual, "|ffffffff")

			aid.Quest = "moo"
			So(aid.DMEncoded(), ShouldEqual, "moo|ffffffff")

			aid.Id = 10
			So(aid.DMEncoded(), ShouldEqual, "moo|fffffff5")

			p, err := aid.ToProperty()
			So(err, ShouldBeNil)
			So(p, ShouldResemble, datastore.MkPropertyNI("moo|fffffff5"))
		})

		Convey("parse", func() {
			Convey("good", func() {
				aid := &Attempt_ID{}
				So(aid.SetDMEncoded("something|ffffffff"), ShouldBeNil)
				So(aid, ShouldResemble, NewAttemptID("something", 0))

				So(aid.FromProperty(datastore.MkPropertyNI("wat|fffffffa")), ShouldBeNil)
				So(aid, ShouldResemble, NewAttemptID("wat", 5))
			})

			Convey("err", func() {
				aid := &Attempt_ID{}
				So(aid.SetDMEncoded("somethingfatsrnt"), ShouldErrLike, "unable to parse")
				So(aid.SetDMEncoded("something|cat"), ShouldErrLike, `strconv.ParseUint: parsing "cat"`)
				So(aid.SetDMEncoded(""), ShouldErrLike, "unable to parse")
				So(aid.SetDMEncoded("somethingffffffff"), ShouldErrLike, "unable to parse Attempt")
				So(aid.FromProperty(datastore.MkPropertyNI(100)), ShouldErrLike, "wrong type")
			})

		})
	})
}

func TestExecutionID(t *testing.T) {
	t.Parallel()

	Convey("Test Execution_ID DM encoding", t, func() {
		Convey("render", func() {
			eid := NewExecutionID("", 0, 0)
			So(eid.DMEncoded(), ShouldEqual, "|ffffffff|ffffffff")

			eid.Quest = "moo"
			So(eid.DMEncoded(), ShouldEqual, "moo|ffffffff|ffffffff")

			eid.Id = 10
			So(eid.DMEncoded(), ShouldEqual, "moo|ffffffff|fffffff5")

			eid.Attempt = 1
			So(eid.DMEncoded(), ShouldEqual, "moo|fffffffe|fffffff5")

			p, err := eid.ToProperty()
			So(err, ShouldBeNil)
			So(p, ShouldResemble, datastore.MkPropertyNI("moo|fffffffe|fffffff5"))
		})

		Convey("parse", func() {
			Convey("good", func() {
				eid := &Execution_ID{}
				So(eid.SetDMEncoded("something|fffffffe|fffffff5"), ShouldBeNil)
				So(eid, ShouldResemble, NewExecutionID("something", 1, 10))

				So(eid.FromProperty(datastore.MkPropertyNI("wat|ffffffff|fffffffa")), ShouldBeNil)
				So(eid, ShouldResemble, NewExecutionID("wat", 0, 5))
			})

			Convey("err", func() {
				eid := &Execution_ID{}
				So(eid.SetDMEncoded("somethingfatsrnt"), ShouldErrLike, "unable to parse")
				So(eid.SetDMEncoded("something|cat|ffffffff|bob"), ShouldErrLike, `strconv.ParseUint: parsing "cat"`)
				So(eid.SetDMEncoded("something|cat|ffffffff"), ShouldErrLike, `strconv.ParseUint: parsing "cat"`)
				So(eid.SetDMEncoded("something|ffffffff|cat"), ShouldErrLike, `strconv.ParseUint: parsing "cat"`)
				So(eid.SetDMEncoded(""), ShouldErrLike, "unable to parse")
				So(eid.SetDMEncoded("somethingffffffff"), ShouldErrLike, "unable to parse")
				So(eid.FromProperty(datastore.MkPropertyNI(100)), ShouldErrLike, "wrong type")
			})

		})
	})
}
