// Copyright 2026 The LUCI Authors.
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

package chromium

import (
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/version"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseVersionedTag(t *testing.T) {
	t.Parallel()

	ftt.Run("ParseVersionedTag", t, func(t *ftt.Test) {
		t.Run("valid 4-component tags", func(t *ftt.Test) {
			v, ok := ParseVersionedTag("refs/tags/155.0.8046.0")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Match(version.Version{
				Major: 155, Minor: 0, Patch: 8046, Build: 0,
			}))

			v, ok = ParseVersionedTag("refs/tags/150.0.7000.1")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Match(version.Version{
				Major: 150, Minor: 0, Patch: 7000, Build: 1,
			}))
		})

		t.Run("valid 3-component tag", func(t *ftt.Test) {
			v, ok := ParseVersionedTag("refs/tags/1.2.3")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Match(version.Version{
				Major: 1, Minor: 2, Patch: 3, Build: 0,
			}))
		})

		t.Run("non-tag refs", func(t *ftt.Test) {
			_, ok := ParseVersionedTag("refs/heads/main")
			assert.Loosely(t, ok, should.BeFalse)

			_, ok = ParseVersionedTag("refs/branch-heads/7000")
			assert.Loosely(t, ok, should.BeFalse)

			_, ok = ParseVersionedTag("HEAD")
			assert.Loosely(t, ok, should.BeFalse)

			_, ok = ParseVersionedTag("")
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("invalid version strings with tag prefix", func(t *ftt.Test) {
			_, ok := ParseVersionedTag("refs/tags/")
			assert.Loosely(t, ok, should.BeFalse)

			_, ok = ParseVersionedTag("refs/tags/beta")
			assert.Loosely(t, ok, should.BeFalse)

			_, ok = ParseVersionedTag("refs/tags/1.2.3.4.5")
			assert.Loosely(t, ok, should.BeFalse)
		})
	})
}

func TestGetBuildVersion(t *testing.T) {
	t.Parallel()

	ftt.Run("GetBuildVersion", t, func(t *ftt.Test) {
		t.Run("nil build", func(t *ftt.Test) {
			_, ok := GetBuildVersion(nil)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("from output gitiles commit", func(t *ftt.Test) {
			b := &buildbucketpb.Build{
				Output: &buildbucketpb.Build_Output{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Ref: "refs/tags/151.0.7001.0",
					},
				},
			}
			v, ok := GetBuildVersion(b)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v.Major, should.Equal(uint64(151)))
		})

		t.Run("from input gitiles commit fallback", func(t *ftt.Test) {
			b := &buildbucketpb.Build{
				Input: &buildbucketpb.Build_Input{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Ref: "refs/tags/150.0.7000.0",
					},
				},
			}
			v, ok := GetBuildVersion(b)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v.Major, should.Equal(uint64(150)))
		})

		t.Run("non-versioned ref", func(t *ftt.Test) {
			b := &buildbucketpb.Build{
				Input: &buildbucketpb.Build_Input{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Ref: "refs/heads/main",
					},
				},
			}
			_, ok := GetBuildVersion(b)
			assert.Loosely(t, ok, should.BeFalse)
		})
	})
}

func TestFindBaselineBuild(t *testing.T) {
	t.Parallel()

	ftt.Run("FindBaselineBuild", t, func(t *ftt.Test) {
		makeBuild := func(
			id int64,
			tag string,
			status buildbucketpb.Status,
		) *buildbucketpb.Build {
			return &buildbucketpb.Build{
				Id:     id,
				Status: status,
				Input: &buildbucketpb.Build_Input{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Ref: tag,
					},
				},
			}
		}

		t.Run("same branch prefers latest build on same branch", func(t *ftt.Test) {
			// Builds on branch 7000:
			// #103: 150.0.7000.4 (SUCCESS)
			// #102: 150.0.7000.2 (SUCCESS)
			// #101: 150.0.7000.0 (SUCCESS)
			// Cross-branch build:
			// #100: 149.0.6999.0 (SUCCESS)
			builds := []*buildbucketpb.Build{
				makeBuild(103, "refs/tags/150.0.7000.4", buildbucketpb.Status_SUCCESS),
				makeBuild(102, "refs/tags/150.0.7000.2", buildbucketpb.Status_SUCCESS),
				makeBuild(101, "refs/tags/150.0.7000.0", buildbucketpb.Status_SUCCESS),
				makeBuild(100, "refs/tags/149.0.6999.0", buildbucketpb.Status_SUCCESS),
			}

			// For 150.0.7000.5, should pick #103 (150.0.7000.4) on same branch.
			b, ok := FindBaselineBuild(
				version.MustParse("150.0.7000.5"),
				builds,
			)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, b.Id, should.Equal(103))
		})

		t.Run("cross-branch ignores non-zero patch builds", func(t *ftt.Test) {
			// Older branch 7000 received patch .25 after 7001 was branched:
			// #103: 150.0.7000.25 (SUCCESS) - cross-branch, non-.0 patch
			// #102: 150.0.7000.0  (SUCCESS) - cross-branch branch point (.0)
			// #101: 149.0.6999.0  (SUCCESS) - older branch point (.0)
			builds := []*buildbucketpb.Build{
				makeBuild(103, "refs/tags/150.0.7000.25", buildbucketpb.Status_SUCCESS),
				makeBuild(102, "refs/tags/150.0.7000.0", buildbucketpb.Status_SUCCESS),
				makeBuild(101, "refs/tags/149.0.6999.0", buildbucketpb.Status_SUCCESS),
			}

			// For 151.0.7001.0 (first build on 7001), must ignore #103
			// and select #102 (150.0.7000.0).
			b, ok := FindBaselineBuild(
				version.MustParse("151.0.7001.0"),
				builds,
			)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, b.Id, should.Equal(102))
		})

		t.Run("cross-branch returns false if no .0 build exists", func(t *ftt.Test) {
			// Only non-.0 builds exist on older branch:
			builds := []*buildbucketpb.Build{
				makeBuild(103, "refs/tags/150.0.7000.25", buildbucketpb.Status_SUCCESS),
				makeBuild(102, "refs/tags/150.0.7000.1", buildbucketpb.Status_SUCCESS),
			}

			// For 151.0.7001.0, no .0 build available cross-branch.
			_, ok := FindBaselineBuild(
				version.MustParse("151.0.7001.0"),
				builds,
			)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("selects nearest smaller passed version", func(t *ftt.Test) {
			// Builds in reverse chronological order:
			// #102: 150.0.7000.1 (SUCCESS)
			// #101: 151.0.7001.0 (SUCCESS)
			// #100: 150.0.7000.0 (SUCCESS)
			builds := []*buildbucketpb.Build{
				makeBuild(102, "refs/tags/150.0.7000.1", buildbucketpb.Status_SUCCESS),
				makeBuild(101, "refs/tags/151.0.7001.0", buildbucketpb.Status_SUCCESS),
				makeBuild(100, "refs/tags/150.0.7000.0", buildbucketpb.Status_SUCCESS),
			}

			// For 151.0.7002.0, should pick #101 (151.0.7001.0) rather than #102
			b, ok := FindBaselineBuild(
				version.MustParse("151.0.7002.0"),
				builds,
			)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, b.Id, should.Equal(101))
		})

		t.Run("ignores failed builds", func(t *ftt.Test) {
			builds := []*buildbucketpb.Build{
				makeBuild(101, "refs/tags/151.0.7001.0", buildbucketpb.Status_FAILURE),
				makeBuild(100, "refs/tags/150.0.7000.0", buildbucketpb.Status_SUCCESS),
			}

			b, ok := FindBaselineBuild(
				version.MustParse("151.0.7002.0"),
				builds,
			)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, b.Id, should.Equal(100))
		})

		t.Run("no smaller version found", func(t *ftt.Test) {
			builds := []*buildbucketpb.Build{
				makeBuild(99, "refs/tags/151.0.7001.0", buildbucketpb.Status_SUCCESS),
			}

			_, ok := FindBaselineBuild(
				version.MustParse("150.0.7000.0"),
				builds,
			)
			assert.Loosely(t, ok, should.BeFalse)
		})
	})
}
