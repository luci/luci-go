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

package strtag

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func ExampleParse_hierarchical() {
	rawtags := []string{
		"foo:bar",
		"swarming_tag:a:1",
		"swarming_tag:b:2",
	}
	tags := Parse(Parse(rawtags)["swarming_tag"])
	fmt.Println(tags.Get("a"))
	fmt.Println(tags.Get("b"))
	// Output:
	// 1
	// 2
}

func TestTags(t *testing.T) {
	t.Parallel()

	Convey("Parse invalid", t, func() {
		k, v := ParseTag("foo")
		So(k, ShouldEqual, "foo")
		So(v, ShouldEqual, "")
	})

	tags := Tags{
		"b": []string{"1"},
		"a": []string{"2"},
	}

	Convey("Tags", t, func() {
		Convey("FormatTag", func() {
			So(tags.Format(), ShouldResemble, []string{
				"a:2",
				"b:1",
			})
		})
		Convey("Get non-existant", func() {
			So(tags.Get("c"), ShouldEqual, "")
		})
		Convey("Get with empty", func() {
			tags := tags.Copy()
			tags["c"] = nil
			So(tags.Get("c"), ShouldEqual, "")
		})
		Convey("Contains", func() {
			So(tags.Contains("a", "2"), ShouldBeTrue)
			So(tags.Contains("a", "1"), ShouldBeFalse)
			So(tags.Contains("b", "1"), ShouldBeTrue)
			So(tags.Contains("c", "1"), ShouldBeFalse)
		})
		Convey("Set", func() {
			tags := tags.Copy()
			tags.Set("c", "3")
			So(tags, ShouldResemble, Tags{
				"b": []string{"1"},
				"a": []string{"2"},
				"c": []string{"3"},
			})
		})
		Convey("Del", func() {
			tags.Del("a")
			So(tags, ShouldResemble, Tags{
				"b": []string{"1"},
			})
		})
	})
}
