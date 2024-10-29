// Copyright 2022 The LUCI Authors.
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

package requirement

import (
	"testing"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestLocationFilterMatch(t *testing.T) {
	ftt.Run("locationFilterMatch works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const refMain = "refs/heads/main"

		// For the purpose of locationFilterMatch, only the Gerrit host,
		// project, ref and paths matter. Parent commits are also taken into
		// consideration when checking if a CL is a merge commit.
		makeCL := func(host, project, ref string, paths []string) *run.RunCL {
			return &run.RunCL{
				ID: common.CLID(1234),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Info: &gerritpb.ChangeInfo{
								Project:         project,
								Ref:             ref,
								CurrentRevision: "deadbeef",
								Revisions: map[string]*gerritpb.RevisionInfo{
									"deadbeef": {
										Commit: &gerritpb.CommitInfo{
											Parents: []*gerritpb.CommitInfo_Parent{
												{Id: "parentSha"},
											},
										},
									},
								},
							},
							Host:  host,
							Files: paths,
						},
					},
				},
			}
		}

		t.Run("with includes and excludes", func(t *ftt.Test) {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				{
					GerritHostRegexp:    "x-review.googlesource.com",
					GerritProjectRegexp: "gp",
					GerritRefRegexp:     refMain,
					PathRegexp:          "included/.*",
					Exclude:             false,
				},
				{
					GerritHostRegexp:    "x-review.googlesource.com",
					GerritProjectRegexp: "gp",
					GerritRefRegexp:     refMain,
					PathRegexp:          "included/excluded/.*",
					Exclude:             true,
				},
			}

			t.Run("in included dir", func(t *ftt.Test) {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/readme.md"})})
				assert.NoErr(t, err)
				assert.Loosely(t, included, should.BeTrue)
			})

			t.Run("in excluded dir", func(t *ftt.Test) {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/excluded/foo"})})
				assert.NoErr(t, err)
				assert.Loosely(t, included, should.BeFalse)
			})

			t.Run("host doesn't match", func(t *ftt.Test) {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("other-review.googlesource.com", "gp", refMain, []string{"included/foo"})})
				assert.NoErr(t, err)
				assert.Loosely(t, included, should.BeFalse)
			})

			t.Run("project doesn't match", func(t *ftt.Test) {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "xyz", refMain, []string{"included/foo"})})
				assert.NoErr(t, err)
				assert.Loosely(t, included, should.BeFalse)
			})

			t.Run("ref doesn't match", func(t *ftt.Test) {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", "refs/heads/experimental", []string{"included/foo"})})
				assert.NoErr(t, err)
				assert.Loosely(t, included, should.BeFalse)
			})

			t.Run("merge commit", func(t *ftt.Test) {
				mergeCL := makeCL("x-review.googlesource.com", "gp", refMain, []string{})
				info := mergeCL.Detail.GetGerrit().GetInfo()
				commit := info.GetRevisions()[info.GetCurrentRevision()].GetCommit()
				commit.Parents = append(commit.Parents, &gerritpb.CommitInfo_Parent{
					Id: "anotherParentSha",
				})

				t.Run("matches host, project, ref", func(t *ftt.Test) {
					included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
					assert.NoErr(t, err)
					assert.Loosely(t, included, should.BeTrue)
				})
				t.Run("doesn't match host", func(t *ftt.Test) {
					mergeCL.Detail.GetGerrit().Host = "other-review.googlesource.com"
					included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
					assert.NoErr(t, err)
					assert.Loosely(t, included, should.BeFalse)
				})
				t.Run("doesn't match project", func(t *ftt.Test) {
					mergeCL.Detail.GetGerrit().GetInfo().Project = "other_proj"
					included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
					assert.NoErr(t, err)
					assert.Loosely(t, included, should.BeFalse)
				})
				t.Run("doesn't match ref", func(t *ftt.Test) {
					mergeCL.Detail.GetGerrit().GetInfo().Project = "other_ref"
					included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
					assert.NoErr(t, err)
					assert.Loosely(t, included, should.BeFalse)
				})
			})

			t.Run("with multiple CLs and multiple files", func(t *ftt.Test) {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{
						makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/readme.md", "included/excluded/foo.txt"}),
						makeCL("x-review.googlesource.com", "foo", refMain, []string{"readme.md"}),
					})
				assert.NoErr(t, err)
				assert.Loosely(t, included, should.BeTrue)
			})
		})

		t.Run("with initial exclude", func(t *ftt.Test) {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				{
					GerritHostRegexp:    "x-review.googlesource.com",
					GerritProjectRegexp: "gp",
					GerritRefRegexp:     refMain,
					PathRegexp:          "excluded/.*",
					Exclude:             true,
				},
			}

			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"excluded/readme.md"})})
			assert.NoErr(t, err)
			assert.Loosely(t, included, should.BeFalse)

			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"somewhere/else"})})
			assert.NoErr(t, err)
			assert.Loosely(t, included, should.BeTrue)
		})

		t.Run("with some patterns empty", func(t *ftt.Test) {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				{
					PathRegexp: "included/.*",
					Exclude:    false,
				},
			}

			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/readme.md"})})
			assert.NoErr(t, err)
			assert.Loosely(t, included, should.BeTrue)

			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("example.com", "blah", refMain, []string{"included/readme.md"})})
			assert.NoErr(t, err)
			assert.Loosely(t, included, should.BeTrue)

			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("example.com", "blah", refMain, []string{"somewhere/else"})})
			assert.NoErr(t, err)
			assert.Loosely(t, included, should.BeFalse)
		})

		t.Run("returns error given invalid regex", func(t *ftt.Test) {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				{
					PathRegexp: "([i*",
				},
			}
			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/readme.md"})})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, included, should.BeFalse)
		})

		t.Run("with nested include exclude include", func(t *ftt.Test) {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				{
					PathRegexp: "included/.*",
					Exclude:    false,
				},
				{
					PathRegexp: "included/excluded/.*",
					Exclude:    true,
				},
				{
					PathRegexp: "included/excluded/included.txt",
					Exclude:    false,
				},
			}

			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/readme.md"})})
			assert.NoErr(t, err)
			assert.Loosely(t, included, should.BeTrue)

			// In excluded dir.
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/excluded/foo"})})
			assert.NoErr(t, err)
			assert.Loosely(t, included, should.BeFalse)

			// In excluded dir, but an exception.
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/excluded/included.txt"})})
			assert.NoErr(t, err)
			assert.Loosely(t, included, should.BeTrue)
		})
	})
}
