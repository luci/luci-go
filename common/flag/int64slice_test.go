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

package flag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInt64SliceFlag(t *testing.T) {
	t.Parallel()

	Convey("one", t, func() {
		var flag int64SliceFlag
		So(flag.Set("1"), ShouldBeNil)
		So(flag.Get(), ShouldResemble, []int64{1})
		So(flag.String(), ShouldEqual, "1")
	})

	Convey("many", t, func() {
		var flag int64SliceFlag
		So(flag.Set("-1"), ShouldBeNil)
		So(flag.Set("0"), ShouldBeNil)
		So(flag.Set("1"), ShouldBeNil)
		So(flag.Get(), ShouldResemble, []int64{-1, 0, 1})
		So(flag.String(), ShouldEqual, "-1, 0, 1")
	})

	Convey("error", t, func() {
		var flag int64SliceFlag
		So(flag.Set("0x00"), ShouldErrLike, "values must be 64-bit integers")
		So(flag, ShouldBeEmpty)
		So(flag, ShouldBeEmpty)
		So(flag.Get(), ShouldBeEmpty)
		So(flag.String(), ShouldBeEmpty)
	})

	Convey("mixed", t, func() {
		var flag int64SliceFlag
		So(flag.Set("-1"), ShouldBeNil)
		So(flag.Set("0x00"), ShouldErrLike, "values must be 64-bit integers")
		So(flag.Set("1"), ShouldBeNil)
		So(flag.Get(), ShouldResemble, []int64{-1, 1})
		So(flag.String(), ShouldEqual, "-1, 1")
	})
}

func TestInt64Slice(t *testing.T) {
	t.Parallel()

	Convey("one", t, func() {
		var i []int64
		So(Int64Slice(&i).Set("1"), ShouldBeNil)
		So(i, ShouldResemble, []int64{1})
	})

	Convey("many", t, func() {
		var i []int64
		So(Int64Slice(&i).Set("-1"), ShouldBeNil)
		So(Int64Slice(&i).Set("0"), ShouldBeNil)
		So(Int64Slice(&i).Set("1"), ShouldBeNil)
		So(i, ShouldResemble, []int64{-1, 0, 1})
	})

	Convey("error", t, func() {
		var i []int64
		So(Int64Slice(&i).Set("0x00"), ShouldErrLike, "values must be 64-bit integers")
		So(i, ShouldBeEmpty)
	})

	Convey("mixed", t, func() {
		var i []int64
		So(Int64Slice(&i).Set("-1"), ShouldBeNil)
		So(Int64Slice(&i).Set("0x00"), ShouldErrLike, "values must be 64-bit integers")
		So(Int64Slice(&i).Set("1"), ShouldBeNil)
		So(i, ShouldResemble, []int64{-1, 1})
	})
}
