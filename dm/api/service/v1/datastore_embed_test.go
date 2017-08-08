// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dm

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/gae/service/datastore"
	. "go.chromium.org/luci/common/testing/assertions"
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
