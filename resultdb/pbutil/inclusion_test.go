// Copyright 2019 The LUCI Authors.
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

package pbutil

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInclusionName(t *testing.T) {
	Convey("ParseInclusionName", t, func() {
		Convey("Parse", func() {
			including, included, err := ParseInclusionName("invocations/a/inclusions/b")
			So(err, ShouldBeNil)
			So(including, ShouldEqual, "a")
			So(included, ShouldEqual, "b")
		})

		Convey("Invalid name", func() {
			_, _, err := ParseInclusionName("invocations/a/inclusions")
			So(err, ShouldErrLike, `does not match`)
		})

		Convey("Format", func() {
			So(InclusionName("a", "b"), ShouldEqual, "invocations/a/inclusions/b")
		})
	})
}
