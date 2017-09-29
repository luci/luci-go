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

package strpair

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func ExampleParse_hierarchical() {
	pairs := []string{
		"foo:bar",
		"swarming_tag:a:1",
		"swarming_tag:b:2",
	}
	tags := ParseMap(ParseMap(pairs)["swarming_tag"])
	fmt.Println(tags.Get("a"))
	fmt.Println(tags.Get("b"))
	// Output:
	// 1
	// 2
}

func TestPairs(t *testing.T) {
	t.Parallel()

	Convey("ParseMap invalid", t, func() {
		k, v := Parse("foo")
		So(k, ShouldEqual, "foo")
		So(v, ShouldEqual, "")
	})

	m := Map{
		"b": []string{"1"},
		"a": []string{"2"},
	}

	Convey("Map", t, func() {
		Convey("Format", func() {
			So(m.Format(), ShouldResemble, []string{
				"a:2",
				"b:1",
			})
		})
		Convey("Get non-existant", func() {
			So(m.Get("c"), ShouldEqual, "")
		})
		Convey("Get with empty", func() {
			m := m.Copy()
			m["c"] = nil
			So(m.Get("c"), ShouldEqual, "")
		})
		Convey("Contains", func() {
			So(m.Contains("a", "2"), ShouldBeTrue)
			So(m.Contains("a", "1"), ShouldBeFalse)
			So(m.Contains("b", "1"), ShouldBeTrue)
			So(m.Contains("c", "1"), ShouldBeFalse)
		})
		Convey("Set", func() {
			m := m.Copy()
			m.Set("c", "3")
			So(m, ShouldResemble, Map{
				"b": []string{"1"},
				"a": []string{"2"},
				"c": []string{"3"},
			})
		})
		Convey("Del", func() {
			m.Del("a")
			So(m, ShouldResemble, Map{
				"b": []string{"1"},
			})
		})
	})
}
