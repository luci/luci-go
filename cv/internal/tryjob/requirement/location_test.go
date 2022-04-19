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

	"go.chromium.org/luci/common/proto/gerrit"

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
		ctx, cancel := ct.SetUp()
		defer cancel()

		// For the purpose of locationFilterMatch, only the Gerrit
		// host, project and paths matter.
		makeCL := func(host, project string, paths []string) *run.RunCL {
			return &run.RunCL{
				ID: common.CLID(1234),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{Info: &gerrit.ChangeInfo{Project: project}, Host: host, Files: paths},
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

			included, err := locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"included/readme.md"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)

			// In excluded dir.
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{"included/excluded/foo"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeFalse)

			// Host doesn't match
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("other-review.googlesource.com", "gp", []string{"included/foo"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeFalse)

			// Project doesn't match
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "xyz", []string{"included/foo"})})
			So(err, ShouldBeNil)
			So(included, ShouldBeFalse)

			// Empty paths, assumed merge commit
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{makeCL("x-review.googlesource.com", "gp", []string{})})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)

			// With multiple CLs and multiple files
			included, err = locationFilterMatch(ctx, lfs,
				[]*run.RunCL{
					makeCL("x-review.googlesource.com", "gp", []string{"included/readme.md", "included/excluded/foo.txt"}),
					makeCL("x-review.googlesource.com", "foo", []string{"readme.md"}),
				})
			So(err, ShouldBeNil)
			So(included, ShouldBeTrue)
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
