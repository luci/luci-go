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

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLocationFilterMatch(t *testing.T) {
	Convey("locationFilterMatch works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

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

		Convey("with includes and excludes", func() {
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

			Convey("in included dir", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/readme.md"})})
				So(err, ShouldBeNil)
				So(included, ShouldBeTrue)
			})

			Convey("in excluded dir", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/excluded/foo"})})
				So(err, ShouldBeNil)
				So(included, ShouldBeFalse)
			})

			Convey("host doesn't match", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("other-review.googlesource.com", "gp", refMain, []string{"included/foo"})})
				So(err, ShouldBeNil)
				So(included, ShouldBeFalse)
			})

			Convey("project doesn't match", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "xyz", refMain, []string{"included/foo"})})
				So(err, ShouldBeNil)
				So(included, ShouldBeFalse)
			})

			Convey("ref doesn't match", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", "refs/heads/experimental", []string{"included/foo"})})
				So(err, ShouldBeNil)
				So(included, ShouldBeFalse)
			})

			Convey("merge commit", func() {
				mergeCL := makeCL("x-review.googlesource.com", "gp", refMain, []string{})
				info := mergeCL.Detail.GetGerrit().GetInfo()
				commit := info.GetRevisions()[info.GetCurrentRevision()].GetCommit()
				commit.Parents = append(commit.Parents, &gerritpb.CommitInfo_Parent{
					Id: "anotherParentSha",
				})

				Convey("matches host, project, ref", func() {
					included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
					So(err, ShouldBeNil)
					So(included, ShouldBeTrue)
				})
				Convey("doesn't match host", func() {
					mergeCL.Detail.GetGerrit().Host = "other-review.googlesource.com"
					included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
					So(err, ShouldBeNil)
					So(included, ShouldBeFalse)
				})
				Convey("doesn't match project", func() {
					mergeCL.Detail.GetGerrit().GetInfo().Project = "other_proj"
					included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
					So(err, ShouldBeNil)
					So(included, ShouldBeFalse)
				})
				Convey("doesn't match ref", func() {
					mergeCL.Detail.GetGerrit().GetInfo().Project = "other_ref"
					included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
					So(err, ShouldBeNil)
					So(included, ShouldBeFalse)
				})
			})

			Convey("with multiple CLs and multiple files", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{
						makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/readme.md", "included/excluded/foo.txt"}),
						makeCL("x-review.googlesource.com", "foo", refMain, []string{"readme.md"}),
					})
				So(err, ShouldBeNil)
				So(included, ShouldBeTrue)
			})
		})

		Convey("with initial exclude", func() {
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
			So(err, ShouldBeNil)
			So(included, ShouldBeFalse)

			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"somewhere/else"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)
		})

		Convey("with some patterns empty", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				{
					PathRegexp: "included/.*",
					Exclude:    false,
				},
			}

			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/readme.md"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)

			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("example.com", "blah", refMain, []string{"included/readme.md"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)

			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("example.com", "blah", refMain, []string{"somewhere/else"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeFalse)
		})

		Convey("returns error given invalid regex", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				{
					PathRegexp: "([i*",
				},
			}
			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/readme.md"})})
			So(err, ShouldNotBeNil)
			So(included, ShouldBeFalse)
		})

		Convey("with nested include exclude include", func() {
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
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)

			// In excluded dir.
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/excluded/foo"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeFalse)

			// In excluded dir, but an exception.
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", refMain, []string{"included/excluded/included.txt"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)
		})
	})
}
