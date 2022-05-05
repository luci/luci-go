// Copyright 2016 The LUCI Authors.
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

func TestStringMap(t *testing.T) {
	Convey(`StringMap`, t, func() {
		m := map[string]string{}
		f := StringMap(m)

		Convey(`Set`, func() {
			So(f.Set("a:1"), ShouldBeNil)
			So(m, ShouldResemble, map[string]string{"a": "1"})

			So(f.Set("b:2"), ShouldBeNil)
			So(m, ShouldResemble, map[string]string{"a": "1", "b": "2"})

			So(f.Set("b:3"), ShouldErrLike, `key "b" is already specified`)

			So(f.Set("c:something:with:colon"), ShouldBeNil)
			So(m, ShouldResemble, map[string]string{"a": "1", "b": "2", "c": "something:with:colon"})
		})

		Convey(`String`, func() {
			m["a"] = "1"
			So(f.String(), ShouldEqual, "a:1")

			m["b"] = "2"
			So(f.String(), ShouldEqual, "a:1 b:2")
		})

		Convey(`Value`, func() {
			So(f.Get(), ShouldEqual, m)
		})
	})
}
