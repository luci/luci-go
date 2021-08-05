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
}
