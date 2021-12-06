// Copyright 2021 The LUCI Authors.
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

package tryjob

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestExternalID(t *testing.T) {
	t.Parallel()

	Convey("ExternalID works", t, func() {

		Convey("BuildbucketID", func() {
			eid, err := BuildbucketID("cr-buildbucket.appspot.com", 12)
			So(err, ShouldBeNil)
			So(eid, ShouldResemble, ExternalID("buildbucket/cr-buildbucket.appspot.com/12"))

			host, build, err := eid.ParseBuildbucketID()
			So(err, ShouldBeNil)
			So(host, ShouldResemble, "cr-buildbucket.appspot.com")
			So(build, ShouldEqual, 12)

			So(eid.MustURL(), ShouldResemble, "https://cr-buildbucket.appspot.com/build/12")
		})
		Convey("Bad ID", func() {
			e := ExternalID("blah")
			_, _, err := e.ParseBuildbucketID()
			So(err, ShouldErrLike, "not a valid BuildbucketID")

			_, err = e.URL()
			So(err, ShouldErrLike, "invalid ExternalID")
		})
	})
	Convey("Resolve works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		host := "example.com"

		Convey("None exist", func() {
			ids := []ExternalID{
				MustBuildbucketID(host, 101),
				MustBuildbucketID(host, 102),
				MustBuildbucketID(host, 103),
			}
			// None of ids[:] are created.

			tjs, err := Resolve(ctx, ids...)
			So(err, ShouldBeNil)
			So(tjs, ShouldHaveLength, 3)
			So(tjs[0], ShouldEqual, 0)
			So(tjs[1], ShouldEqual, 0)
			So(tjs[2], ShouldEqual, 0)
		})
		Convey("Some exist", func() {
			ids := []ExternalID{
				MustBuildbucketID(host, 201),
				MustBuildbucketID(host, 202),
				MustBuildbucketID(host, 203),
			}
			// ids[0] is not created.
			ids[1].MustCreateIfNotExists(ctx)
			ids[2].MustCreateIfNotExists(ctx)

			tjs, err := Resolve(ctx, ids...)
			So(err, ShouldBeNil)
			So(tjs, ShouldHaveLength, 3)
			So(tjs[0], ShouldEqual, 0)
			So(tjs[1], ShouldNotEqual, 0)
			So(tjs[2], ShouldNotEqual, 0)
		})

		Convey("All exist", func() {
			ids := []ExternalID{
				MustBuildbucketID(host, 301),
				MustBuildbucketID(host, 302),
				MustBuildbucketID(host, 303),
			}
			ids[0].MustCreateIfNotExists(ctx)
			ids[1].MustCreateIfNotExists(ctx)
			ids[2].MustCreateIfNotExists(ctx)

			tjs, err := Resolve(ctx, ids...)
			So(err, ShouldBeNil)
			So(tjs, ShouldHaveLength, 3)
			So(tjs[0], ShouldNotEqual, 0)
			So(tjs[1], ShouldNotEqual, 0)
			So(tjs[2], ShouldNotEqual, 0)
		})
	})
}
