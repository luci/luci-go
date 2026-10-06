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

// Package chromium provides utilities for resolving commit regression ranges
// and blamelists for Chromium release tags across LUCI services.
package chromium

import (
	"strings"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/version"
)

const versionedTagPrefix = "refs/tags/"

// ParseVersionedTag extracts and parses a Version from a git ref if it has the
// "refs/tags/" prefix (refs/tags/X.Y.Z.W). Returns (Version{}, false) if ref
// does not start with "refs/tags/" or is not a valid version string.
func ParseVersionedTag(ref string) (version.Version, bool) {
	versionStr, ok := strings.CutPrefix(ref, versionedTagPrefix)
	if !ok {
		return version.Version{}, false
	}
	v, err := version.Parse(versionStr)
	if err != nil {
		return version.Version{}, false
	}
	return v, true
}

// GetBuildVersion extracts the Version from a build's output or input
// gitiles commit.
func GetBuildVersion(b *buildbucketpb.Build) (version.Version, bool) {
	ref := b.GetOutput().GetGitilesCommit().GetRef()
	if ref == "" {
		ref = b.GetInput().GetGitilesCommit().GetRef()
	}
	return ParseVersionedTag(ref)
}

// FindBaselineBuild scans builds and returns the baseline build for
// targetVersion following the 4-part release version convention (e.g.
// Chromium release tags refs/tags/MAJOR.MINOR.BUILD.PATCH):
//  1. If there are passing builds on the same branch, it picks the latest
//     (largest version strictly smaller than targetVersion on that branch).
//  2. Otherwise, it falls back to cross-branch comparison and picks the
//     nearest smaller branch-point build (ending in .0).
func FindBaselineBuild(
	targetVersion version.Version,
	builds []*buildbucketpb.Build,
) (*buildbucketpb.Build, bool) {
	var bestBuild *buildbucketpb.Build
	var bestVersion version.Version
	sameBranch := false

	for _, b := range builds {
		if b.GetStatus() != buildbucketpb.Status_SUCCESS {
			continue
		}
		v, ok := GetBuildVersion(b)
		if !ok || !v.Less(targetVersion) {
			continue
		}

		if isSameBranch(v, targetVersion) {
			if !sameBranch || bestVersion.Less(v) {
				bestBuild = b
				bestVersion = v
				sameBranch = true
			}
		} else if !sameBranch && isBranchPoint(v) {
			if bestBuild == nil || bestVersion.Less(v) {
				bestBuild = b
				bestVersion = v
			}
		}
	}

	if bestBuild == nil {
		return nil, false
	}
	return bestBuild, true
}

// isSameBranch reports whether two versions share the same branch.
// In Chromium's MAJOR.MINOR.BUILD.PATCH convention, the 3rd component (BUILD)
// is the release branch number (e.g. 7000 in refs/branch-heads/7000), while
// the 4th component (PATCH) is the patch number on that branch. We check that
// the milestone prefix and branch number match.
func isSameBranch(a, b version.Version) bool {
	return a.Major == b.Major && a.Minor == b.Minor && a.Patch == b.Patch
}

// isBranchPoint reports whether the version represents a branch point.
// In Chromium's MAJOR.MINOR.BUILD.PATCH convention, the 4th component (PATCH)
// being 0 represents the initial branch cut from trunk.
func isBranchPoint(v version.Version) bool {
	return v.Build == 0
}
