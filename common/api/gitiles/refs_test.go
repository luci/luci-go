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

package gitiles

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRefs(t *testing.T) {
	t.Parallel()

	Convey("RefSet works", t, func() {
		wr := NewRefSet([]string{
			`refs/heads/master`,
			`regexp:refs/branch-heads/\d+\.\d+`,
		})

		Convey("explicit refs work", func() {
			So(wr.Has("refs/heads/master"), ShouldBeTrue)
			So(wr.Has("refs/heads/foo"), ShouldBeFalse)
		})

		Convey("regexp refs work", func() {
			So(wr.Has("refs/branch-heads/1.12"), ShouldBeTrue)
			So(wr.Has("refs/branch-heads/1.12.123"), ShouldBeFalse)
		})

		Convey("ForEachPrefix works", func() {
			namespaces := []string{}
			wr.ForEachPrefix(func(refsPath string) {
				namespaces = append(namespaces, refsPath)
			})
			So("refs/heads", ShouldBeIn, namespaces)
			So("refs/branch-heads", ShouldBeIn, namespaces)
		})
	})

	Convey("SplitRef works", t, func() {
		p, s := SplitRef("refs/heads/master")
		So(p, ShouldEqual, "refs/heads")
		So(s, ShouldEqual, "master")
		p, s = SplitRef("refs/weird/")
		So(p, ShouldEqual, "refs/weird")
		So(s, ShouldEqual, "")
		p, s = SplitRef("refs")
		So(p, ShouldEqual, "refs")
		So(s, ShouldBeEmpty)
	})
}
