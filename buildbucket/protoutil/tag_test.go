// Copyright 2019 The LUCI Authors.
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

package protoutil

import (
	"testing"

	"go.chromium.org/luci/common/data/strpair"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuildSet(t *testing.T) {
	t.Parallel()

	Convey("Gerrit", t, func() {
		Convey("ParseBuildSet", func() {
			actual := ParseBuildSet("patch/gerrit/chromium-review.googlesource.com/678507/3")
			So(actual, ShouldResemble, &pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   678507,
				Patchset: 3,
			})
		})
		Convey("BuildSet", func() {
			bs := GerritBuildSet(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   678507,
				Patchset: 3,
			})
			So(bs, ShouldEqual, "patch/gerrit/chromium-review.googlesource.com/678507/3")
		})
	})

	Convey("Gitiles", t, func() {
		Convey("ParseBuildSet", func() {
			actual := ParseBuildSet("commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7")
			So(actual, ShouldResemble, &pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Id:      "b7a757f457487cd5cfe2dae83f65c5bc10e288b7",
			})
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
		Convey("BuildSet", func() {
			bs := GitilesBuildSet(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Id:      "b7a757f457487cd5cfe2dae83f65c5bc10e288b7",
			})
			So(bs, ShouldEqual, "commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7")
		})
	})
}

func TestStringPairs(t *testing.T) {
	t.Parallel()

	Convey("StringPairs", t, func() {
		m := strpair.Map{}
		m.Add("a", "1")
		m.Add("a", "2")
		m.Add("b", "1")

		pairs := StringPairs(m)
		So(pairs, ShouldResembleProto, []*pb.StringPair{
			{Key: "a", Value: "1"},
			{Key: "a", Value: "2"},
			{Key: "b", Value: "1"},
		})
	})
}
