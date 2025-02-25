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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/changelist"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
)

func TestRelatedChangeProcessing(t *testing.T) {
	t.Parallel()

	ftt.Run("setGitDeps works", t, func(t *ftt.Test) {
		ctx := context.Background()
		f := fetcher{
			change: 111,
			host:   "host",
			toUpdate: changelist.UpdateFields{
				Snapshot: &changelist.Snapshot{Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{}}},
			},
		}

		t.Run("No related changes", func(t *ftt.Test) {
			f.setGitDeps(ctx, nil)
			assert.Loosely(t, f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), should.BeNil)

			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{})
			assert.Loosely(t, f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), should.BeNil)
		})

		t.Run("Just itself", func(t *ftt.Test) {
			// This isn't happening today, but CV shouldn't choke if Gerrit changes.
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 3, 3), // No parents.
			})
			assert.Loosely(t, f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), should.BeNil)

			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 3, 3, "107_2"),
			})
			assert.Loosely(t, f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), should.BeNil)
		})

		t.Run("Has related, but no deps", func(t *ftt.Test) {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 3, 3, "107_2"),
				gf.RelatedChange(114, 1, 3, "111_3"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			assert.Loosely(t, f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), should.BeNil)
		})

		t.Run("Has related, but lacking this change crbug/1199471", func(t *ftt.Test) {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(114, 1, 3, "111_3"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			assert.Loosely(t, f.toUpdate.Snapshot.GetErrors(), should.HaveLength(1))
			assert.Loosely(t, f.toUpdate.Snapshot.GetErrors()[0].GetCorruptGerritMetadata(), should.ContainSubstring("https://crbug.com/1199471"))
		})

		t.Run("Has related, and several times itself", func(t *ftt.Test) {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(111, 2, 2, "107_2"),
				gf.RelatedChange(111, 3, 3, "107_2"),
				gf.RelatedChange(114, 1, 3, "111_3"),
			})
			assert.Loosely(t, f.toUpdate.Snapshot.GetErrors()[0].GetCorruptGerritMetadata(), should.ContainSubstring("https://crbug.com/1199471"))
		})

		t.Run("1 parent", func(t *ftt.Test) {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(107, 1, 3, "104_2"),
				gf.RelatedChange(111, 3, 3, "107_1"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			assert.Loosely(t, f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), should.Match([]*changelist.GerritGitDep{
				{Change: 107, Immediate: true},
			}))
		})

		t.Run("Diamond", func(t *ftt.Test) {
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(103, 2, 2),
				gf.RelatedChange(104, 2, 2, "103_2"),
				gf.RelatedChange(107, 1, 3, "104_2"),
				gf.RelatedChange(108, 1, 3, "104_2"),
				gf.RelatedChange(111, 3, 3, "107_1", "108_1"),
				gf.RelatedChange(114, 1, 3, "111_3"),
				gf.RelatedChange(117, 2, 2, "114_1"),
			})
			assert.Loosely(t, f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), should.Match([]*changelist.GerritGitDep{
				{Change: 107, Immediate: true},
				{Change: 108, Immediate: true},
				{Change: 104, Immediate: false},
				{Change: 103, Immediate: false},
			}))
		})

		t.Run("Same revision, different changes", func(t *ftt.Test) {
			c104 := gf.RelatedChange(104, 1, 1, "103_2")
			c105 := gf.RelatedChange(105, 1, 1, "103_2")
			c105.GetCommit().Id = c104.GetCommit().GetId()
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(103, 2, 2),
				c104,
				c105, // should be ignored, somewhat arbitrarily.
				gf.RelatedChange(111, 3, 3, "104_1"),
			})
			assert.Loosely(t, f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), should.Match([]*changelist.GerritGitDep{
				{Change: 104, Immediate: true},
				{Change: 103, Immediate: false},
			}))
		})

		t.Run("2 parents which are the same change at different revisions", func(t *ftt.Test) {
			// Actually happened, see https://crbug.com/988309.
			f.setGitDeps(ctx, []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
				gf.RelatedChange(104, 1, 2, "long-ago-merged1"),
				gf.RelatedChange(107, 1, 1, "long-ago-merged2"),
				gf.RelatedChange(104, 2, 2, "107_1"),
				gf.RelatedChange(111, 3, 3, "104_1", "104_2"),
			})
			assert.Loosely(t, f.toUpdate.Snapshot.GetGerrit().GetGitDeps(), should.Match([]*changelist.GerritGitDep{
				{Change: 104, Immediate: true},
				{Change: 107, Immediate: false},
			}))
		})
	})
}
