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

package buildbucket

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuildSet(t *testing.T) {
	t.Parallel()

	Convey("Gerrit", t, func() {
		Convey("ParseMap", func() {
			actual := ParseBuildSet("patch/gerrit/chromium-review.googlesource.com/678507/3")
			So(actual, ShouldResemble, &GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   678507,
				PatchSet: 3,
			})
		})
		Convey("String", func() {
			bs := &GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   678507,
				PatchSet: 3,
			}
			So(bs.String(), ShouldEqual, "patch/gerrit/chromium-review.googlesource.com/678507/3")
		})
	})

	Convey("Gitiles", t, func() {
		Convey("ParseMap", func() {
			actual := ParseBuildSet("commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7")
			So(actual, ShouldResemble, &GitilesCommit{
				Host:     "chromium.googlesource.com",
				Project:  "infra/luci/luci-go",
				Revision: "b7a757f457487cd5cfe2dae83f65c5bc10e288b7",
			})
		})
		Convey("String", func() {
			bs := &GitilesCommit{
				Host:     "chromium.googlesource.com",
				Project:  "infra/luci/luci-go",
				Revision: "b7a757f457487cd5cfe2dae83f65c5bc10e288b7",
			}
			So(bs.String(), ShouldEqual, "commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7")
		})
		Convey("not sha1", func() {
			bs := ParseBuildSet("commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/non-sha1")
			So(bs, ShouldBeNil)
		})

		Convey("no host", func() {
			bs := ParseBuildSet("commit/gitiles//infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7")
			So(bs, ShouldBeNil)
		})

		Convey("no plus", func() {
			bs := ParseBuildSet("commit/gitiles//infra/luci/luci-go/b7a757f457487cd5cfe2dae83f65c5bc10e288b7")
			So(bs, ShouldBeNil)
		})
	})
}
