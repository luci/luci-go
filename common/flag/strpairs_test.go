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
	"flag"
	"testing"

	"go.chromium.org/luci/common/data/strpair"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStrPairs(t *testing.T) {
	t.Parallel()

	Convey("StringPairs", t, func() {
		m := strpair.Map{}
		v := StringPairs(m)

		Convey("Set", func() {
			Convey("Once", func() {
				So(v.Set("a:1"), ShouldBeNil)
				So(m.Format(), ShouldResemble, []string{"a:1"})

				Convey("Second time", func() {
					So(v.Set("b:1"), ShouldBeNil)
					So(m.Format(), ShouldResemble, []string{"a:1", "b:1"})
				})

				Convey("Same key", func() {
					So(v.Set("a:2"), ShouldBeNil)
					So(m.Format(), ShouldResemble, []string{"a:1", "a:2"})
				})
			})
			Convey("No colon", func() {
				So(v.Set("a"), ShouldErrLike, "no colon")
			})
		})

		Convey("String", func() {
			m.Add("a", "1")
			m.Add("a", "2")
			m.Add("b", "1")
			So(v.String(), ShouldEqual, "a:1, a:2, b:1")
		})

		Convey("Get", func() {
			So(v.(flag.Getter).Get(), ShouldEqual, m)
		})
	})
}
