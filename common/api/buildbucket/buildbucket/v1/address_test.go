// Copyright 2018 The LUCI Authors.
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

package buildbucket

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAddress(t *testing.T) {
	t.Parallel()

	Convey("FormatBuildAddress", t, func(c C) {
		Convey("Number", func() {
			So(FormatBuildAddress(1, "luci.chromium.try", "linux-rel", 2), ShouldEqual, "luci.chromium.try/linux-rel/2")
		})
		Convey("ID", func() {
			So(FormatBuildAddress(1, "luci.chromium.try", "linux-rel", 0), ShouldEqual, "1")
		})
	})
	Convey("ParseBuildAddress", t, func(c C) {
		Convey("Number", func() {
			id, project, bucket, builder, number, err := ParseBuildAddress("luci.chromium.try/linux-rel/2")
			So(err, ShouldBeNil)
			So(id, ShouldEqual, 0)
			So(project, ShouldEqual, "chromium")
			So(bucket, ShouldEqual, "luci.chromium.try")
			So(builder, ShouldEqual, "linux-rel")
			So(number, ShouldEqual, 2)
		})
		Convey("ID", func() {
			id, project, bucket, builder, number, err := ParseBuildAddress("1")
			So(err, ShouldBeNil)
			So(id, ShouldEqual, 1)
			So(project, ShouldEqual, "")
			So(bucket, ShouldEqual, "")
			So(builder, ShouldEqual, "")
			So(number, ShouldEqual, 0)
		})
		Convey("Unrecognized", func() {
			_, _, _, _, _, err := ParseBuildAddress("a/b")
			So(err, ShouldNotBeNil)
		})
	})
}
