// Copyright 2020 The LUCI Authors.
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

package filegraph

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestName(t *testing.T) {
	t.Parallel()

	Convey(`Name`, t, func() {
		Convey(`ParseName`, func() {
			Convey(`Works`, func() {
				expected := Name{"a", "b", "c"}
				actual := ParseName("a/b/c")
				So(actual, ShouldResemble, expected)
			})
			Convey(`Root`, func() {
				var expected Name
				actual := ParseName("")
				So(actual, ShouldResemble, expected)
			})
		})
		Convey(`String`, func() {
			Convey(`Works`, func() {
				actual := (Name{"a", "b", "c"}).String()
				So(actual, ShouldEqual, "a/b/c")
			})
			Convey(`Root`, func() {
				actual := (Name)(nil).String()
				So(actual, ShouldEqual, "")
			})
		})
	})
}
