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
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLocationFilterMatch(t *testing.T) {
	Convey("locationFilterMatch works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// For the purpose of locationFilterMatch, only the Gerrit host,
		// project and paths matter. Parent commits are also taken into
		// consideration when checking if a CL is a merge commit.
		makeCL := func(host, project string, paths []string) *run.RunCL {
			return &run.RunCL{
				ID: common.CLID(1234),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Info:  &gerritpb.ChangeInfo{Project: project},
							Host:  host,
							Files: paths,
						},
					},
				},
			}
		}

		Convey("with includes and excludes", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					GerritHostRegexp:    "x-review.googlesource.com",
					GerritProjectRegexp: "gp",
					PathRegexp:          "included/.*",
					Exclude:             false,
				},
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					GerritHostRegexp:    "x-review.googlesource.com",
					GerritProjectRegexp: "gp",
					PathRegexp:          "included/excluded/.*",
					Exclude:             true,
				},
			}

			Convey("in included dir", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"included/readme.md"})})
				So(err, ShouldBeNil)
				So(included, ShouldBeTrue)
			})

			Convey("in excluded dir", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"included/excluded/foo"})})
				So(err, ShouldBeNil)
				So(included, ShouldBeFalse)
			})

			Convey("host doesn't match", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("other-review.googlesource.com", "gp", []string{"included/foo"})})
				So(err, ShouldBeNil)
				So(included, ShouldBeFalse)
			})

			Convey("project doesn't match", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{makeCL("x-review.googlesource.com", "xyz", []string{"included/foo"})})
				So(err, ShouldBeNil)
				So(included, ShouldBeFalse)
			})

			Convey("merge commit: no files changed and two parents", func() {
				mergeCL := makeCL("x-review.googlesource.com", "gp", []string{})
				mergeCL.Detail.GetGerrit().Info = gf.CI(1234, gf.Project("gp"), gf.ParentCommits([]string{"one", "two"}))
				included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
				So(err, ShouldBeNil)
				So(included, ShouldBeTrue)
			})

			Convey("no files changed and one parents", func() {
				// Any empty diff with no files is possible for non-merge-commits.
				// Not treated as merge commit, so builder should not be triggered.
				mergeCL := makeCL("x-review.googlesource.com", "gp", []string{})
				mergeCL.Detail.GetGerrit().Info = gf.CI(1234, gf.Project("gp"), gf.ParentCommits([]string{"one"}))
				included, err := locationFilterMatch(ctx, lfs, []*run.RunCL{mergeCL})
				So(err, ShouldBeNil)
				So(included, ShouldBeFalse)
			})

			Convey("with multiple CLs and multiple files", func() {
				included, err := locationFilterMatch(ctx, lfs,
					[]*run.RunCL{
						makeCL("x-review.googlesource.com", "gp", []string{"included/readme.md", "included/excluded/foo.txt"}),
						makeCL("x-review.googlesource.com", "foo", []string{"readme.md"}),
					})
				So(err, ShouldBeNil)
				So(included, ShouldBeTrue)
			})
		})

		Convey("with initial exclude", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					GerritHostRegexp:    "x-review.googlesource.com",
					GerritProjectRegexp: "gp",
					PathRegexp:          "excluded/.*",
					Exclude:             true,
				},
			}

			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"excluded/readme.md"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeFalse)

			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"somewhere/else"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)
		})

		Convey("with some patterns empty", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					PathRegexp: "included/.*",
					Exclude:    false,
				},
			}

			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"included/readme.md"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)

			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("example.com", "blah", []string{"included/readme.md"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)

			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("example.com", "blah", []string{"somewhere/else"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeFalse)
		})

		Convey("returns error given invalid regex", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					PathRegexp: "([i*",
				},
			}
			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"included/readme.md"})})
			So(err, ShouldNotBeNil)
			So(included, ShouldBeFalse)
		})

		Convey("with nested include exclude include", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					PathRegexp: "included/.*",
					Exclude:    false,
				},
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					PathRegexp: "included/excluded/.*",
					Exclude:    true,
				},
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					PathRegexp: "included/excluded/included.txt",
					Exclude:    false,
				},
			}

			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"included/readme.md"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)

			// In excluded dir.
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"included/excluded/foo"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeFalse)

			// In excluded dir, but an exception.
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"included/excluded/included.txt"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)
		})
	})
}

func TestHelperFunctions(t *testing.T) {
	Convey("isMergeCommit", t, func() {

		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		Convey("returns false on missing data", func() {
			So(isMergeCommit(ctx, nil), ShouldBeFalse)
			So(isMergeCommit(ctx, &changelist.Gerrit{}), ShouldBeFalse)
			So(isMergeCommit(ctx, &changelist.Gerrit{Info: &gerritpb.ChangeInfo{}}), ShouldBeFalse)
			So(isMergeCommit(ctx, &changelist.Gerrit{Info: gf.CI(10)}), ShouldBeFalse)
		})
		Convey("returns true with no files and two parents", func() {
			So(isMergeCommit(
				ctx,
				&changelist.Gerrit{
					Files: []string{},
					Info:  gf.CI(10, gf.ParentCommits([]string{"one", "two"})),
				}), ShouldBeTrue)
		})
		Convey("returns false with no files and one parent", func() {
			So(isMergeCommit(
				ctx,
				&changelist.Gerrit{
					Files: []string{},
					Info:  gf.CI(10, gf.ParentCommits([]string{"one"})),
				}), ShouldBeFalse)
		})
	})

	Convey("hostAndProjectMatch", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		Convey("when only some files in a repo are included", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					GerritHostRegexp:    "x-review.googlesource.com",
					GerritProjectRegexp: "gp",
					PathRegexp:          "included/.*",
					Exclude:             false,
				},
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					GerritHostRegexp:    "x-review.googlesource.com",
					GerritProjectRegexp: "gp",
					PathRegexp:          "included/excluded/.*",
					Exclude:             true,
				},
			}
			compiled, err := compileLocationFilters(ctx, lfs)
			So(err, ShouldBeNil)
			So(hostAndProjectMatch(compiled, "x-review.googlesource.com", "gp"), ShouldBeTrue)
			So(hostAndProjectMatch(compiled, "x-review.googlesource.com", "other"), ShouldBeFalse)
		})

		Convey("simple match all case", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					GerritHostRegexp:    ".*",
					GerritProjectRegexp: ".*",
					PathRegexp:          "foo/.*",
					Exclude:             false,
				},
			}
			compiled, err := compileLocationFilters(ctx, lfs)
			So(err, ShouldBeNil)
			So(hostAndProjectMatch(compiled, "x-review.googlesource.com", "gp"), ShouldBeTrue)
		})

		Convey("default include case", func() {
			lfs := []*cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
				&cfgpb.Verifiers_Tryjob_Builder_LocationFilter{
					GerritHostRegexp:    ".*",
					GerritProjectRegexp: ".*",
					PathRegexp:          "foo/.*",
					Exclude:             true,
				},
			}
			compiled, err := compileLocationFilters(ctx, lfs)
			So(err, ShouldBeNil)
			So(hostAndProjectMatch(compiled, "x-review.googlesource.com", "gp"), ShouldBeTrue)
		})
	})
}
