// Copyright 2017 The LUCI Authors.
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

package int64listflag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInt64ListFlag(t *testing.T) {
	Convey("one", t, func() {
		flag := Flag{}
		So(flag.Set("1"), ShouldBeNil)
		So(flag, ShouldHaveLength, 1)
		So(flag[0], ShouldEqual, 1)
		So(flag.String(), ShouldEqual, "1")
	})

	Convey("many", t, func() {
		flag := Flag{}
		So(flag.Set("-1"), ShouldBeNil)
		So(flag.Set("0"), ShouldBeNil)
		So(flag.Set("1"), ShouldBeNil)
		So(flag, ShouldHaveLength, 3)
		So(flag[0], ShouldEqual, -1)
		So(flag[1], ShouldEqual, 0)
		So(flag[2], ShouldEqual, 1)
		So(flag.String(), ShouldEqual, "-1, 0, 1")
	})

	Convey("error", t, func() {
		flag := Flag{}
		So(flag.Set("0x00"), ShouldErrLike, "values must be 64-bit integers")
		So(flag, ShouldBeEmpty)
		So(flag.String(), ShouldBeEmpty)
	})

	Convey("mixed", t, func() {
		flag := Flag{}
		So(flag.Set("-1"), ShouldBeNil)
		So(flag.Set("0x00"), ShouldErrLike, "values must be 64-bit integers")
		So(flag.Set("1"), ShouldBeNil)
		So(flag, ShouldHaveLength, 2)
		So(flag.String(), ShouldEqual, "-1, 1")
	})
}
