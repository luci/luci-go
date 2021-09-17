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

package changelist

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExternalID(t *testing.T) {
	t.Parallel()

	Convey("ExternalID works", t, func() {

		Convey("GobID", func() {
			eid, err := GobID("x-review.example.com", 12)
			So(err, ShouldBeNil)
			So(eid, ShouldResemble, ExternalID("gerrit/x-review.example.com/12"))

			host, change, err := eid.ParseGobID()
			So(err, ShouldBeNil)
			So(host, ShouldResemble, "x-review.example.com")
			So(change, ShouldEqual, 12)

			So(eid.MustURL(), ShouldResemble, "https://x-review.example.com/c/12")
		})

		Convey("Invalid GobID", func() {
			_, _, err := ExternalID("meh").ParseGobID()
			So(err, ShouldErrLike, "is not a valid GobID")

			_, _, err = ExternalID("gerrit/x/y").ParseGobID()
			So(err, ShouldErrLike, "is not a valid GobID")
		})

	})
}
