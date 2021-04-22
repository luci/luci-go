// Copyright 2021 The LUCI Authors.
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

package invocations

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLongestCommonPrefix(t *testing.T) {
	t.Parallel()
	Convey("empty", t, func() {
		So(LongestCommonPrefix("", "str"), ShouldEqual, "")
	})

	Convey("no common prefix", t, func() {
		So(LongestCommonPrefix("str", "other"), ShouldEqual, "")
	})

	Convey("common prefix", t, func() {
		So(LongestCommonPrefix("prefix_1", "prefix_2"), ShouldEqual, "prefix_")
	})
}
