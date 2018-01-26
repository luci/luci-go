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

package int32flag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInt32Flag(t *testing.T) {
	Convey("error", t, func() {
		var flag Flag
		So(flag.Set("0x01"), ShouldErrLike, "values must be 32-bit integers")
		So(flag, ShouldEqual, 0)
		So(flag.Get(), ShouldEqual, 0)
		So(flag.String(), ShouldEqual, "0")
	})

	Convey("int64", t, func() {
		var flag Flag
		So(flag.Set("2147483648"), ShouldErrLike, "values must be 32-bit integers")
		So(flag, ShouldEqual, 0)
		So(flag.Get(), ShouldEqual, 0)
		So(flag.String(), ShouldEqual, "0")
	})

	Convey("zero", t, func() {
		var flag Flag
		So(flag.Set("0"), ShouldBeNil)
		So(flag, ShouldEqual, 0)
		So(flag.Get(), ShouldEqual, 0)
		So(flag.String(), ShouldEqual, "0")
	})

	Convey("min", t, func() {
		var flag Flag
		So(flag.Set("-2147483648"), ShouldBeNil)
		So(flag, ShouldEqual, -2147483648)
		So(flag.Get(), ShouldEqual, -2147483648)
		So(flag.String(), ShouldEqual, "-2147483648")
	})

	Convey("max", t, func() {
		var flag Flag
		So(flag.Set("2147483647"), ShouldBeNil)
		So(flag, ShouldEqual, 2147483647)
		So(flag.Get(), ShouldEqual, 2147483647)
		So(flag.String(), ShouldEqual, "2147483647")
	})
}
