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

package updater

import (
	"context"
	"testing"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/cv/internal/changelist"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRelatedChangeProcessing(t *testing.T) {
	t.Parallel()

	Convey("setGitDeps works", t, func() {
		ctx := context.Background()
		f := fetcher{
			change: 111,
			host:   "host",
			toUpdate: changelist.UpdateFields{
				Snapshot: &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{}}},
			},
		}

		Convey("No related changes", func() {
			f.setGitDeps(ctx, nil)
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)

			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)
		})

		Convey("Just itself", func() {
			// This isn't happening today, but CV shouldn't choke if Gerrit changes.
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 3, 3), // No parents.
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)

			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 3, 3, "107_2"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)
		})

		Convey("Has related, but no deps", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 3, 3, "107_2"),
				gf.RelatedChange(114, 1, 3, "111_3"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldBeNil)
		})

		Convey("Has related, but lacking this change crbug/1199471", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(114, 1, 3, "111_3"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			So(f.toUpdate.Snapshot.GetErrors(), ShouldHaveLength, 1)
			So(f.toUpdate.Snapshot.GetErrors()[0].GetCorruptGerritMetadata(), ShouldContainSubstring, "https://crbug.com/1199471")
		})

		Convey("Has related, and several times itself", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 2, 2, "107_2"),
				gf.RelatedChange(111, 3, 3, "107_2"),
				gf.RelatedChange(114, 1, 3, "111_3"),
			})
			So(f.toUpdate.Snapshot.GetErrors()[0].GetCorruptGerritMetadata(), ShouldContainSubstring, "https://crbug.com/1199471")
		})

		Convey("1 parent", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(107, 1, 3, "104_2"),
				gf.RelatedChange(111, 3, 3, "107_1"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, []*changelist.GerritGitDep{
				{Change: 107, Immediate: true},
			})
		})

		Convey("Diamond", func() {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(103, 2, 2),
				gf.RelatedChange(104, 2, 2, "103_2"),
				gf.RelatedChange(107, 1, 3, "104_2"),
				gf.RelatedChange(108, 1, 3, "104_2"),
				gf.RelatedChange(111, 3, 3, "107_1", "108_1"),
				gf.RelatedChange(114, 1, 3, "111_3"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, []*changelist.GerritGitDep{
				{Change: 107, Immediate: true},
				{Change: 108, Immediate: true},
				{Change: 104, Immediate: false},
				{Change: 103, Immediate: false},
			})
		})

		Convey("Same revision, different changes", func() {
			c104 := gf.RelatedChange(104, 1, 1, "103_2")
			c105 := gf.RelatedChange(105, 1, 1, "103_2")
			c105.GetCommit().Id = c104.GetCommit().GetId()
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(103, 2, 2),
				c104,
				c105, // should be ignored, somewhat arbitrarily.
				gf.RelatedChange(111, 3, 3, "104_1"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, []*changelist.GerritGitDep{
				{Change: 104, Immediate: true},
				{Change: 103, Immediate: false},
			})
		})

		Convey("2 parents which are the same change at different revisions", func() {
			// Actually happened, see https://crbug.com/988309.
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(104, 1, 2, "long-ago-merged1"),
				gf.RelatedChange(107, 1, 1, "long-ago-merged2"),
				gf.RelatedChange(104, 2, 2, "107_1"),
				gf.RelatedChange(111, 3, 3, "104_1", "104_2"),
			})
			So(f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), ShouldResembleProto, []*changelist.GerritGitDep{
				{Change: 104, Immediate: true},
				{Change: 107, Immediate: false},
			})
		})
	})
}
