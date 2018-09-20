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

	"google.golang.org/api/googleapi"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFieldSliceFlag(t *testing.T) {
	t.Parallel()

	Convey("one", t, func() {
		var flag fieldSliceFlag
		So(flag.Set("abc"), ShouldBeNil)
		So(flag.Get(), ShouldResemble, []googleapi.Field{"abc"})
		So(flag.String(), ShouldEqual, "abc")
	})

	Convey("many", t, func() {
		var flag fieldSliceFlag
		So(flag.Set("abc"), ShouldBeNil)
		So(flag.Set("def"), ShouldBeNil)
		So(flag.Set("ghi"), ShouldBeNil)
		So(flag.Get(), ShouldResemble, []googleapi.Field{"abc", "def", "ghi"})
		So(flag.String(), ShouldEqual, "abc, def, ghi")
	})
}

func TestFieldSlice(t *testing.T) {
	t.Parallel()

	Convey("one", t, func() {
		var f []googleapi.Field
		So(FieldSlice(&f).Set("abc"), ShouldBeNil)
		So(f, ShouldResemble, []googleapi.Field{"abc"})
	})

	Convey("many", t, func() {
		var f []googleapi.Field
		So(FieldSlice(&f).Set("abc"), ShouldBeNil)
		So(FieldSlice(&f).Set("def"), ShouldBeNil)
		So(FieldSlice(&f).Set("ghi"), ShouldBeNil)
		So(f, ShouldResemble, []googleapi.Field{"abc", "def", "ghi"})
	})
}
