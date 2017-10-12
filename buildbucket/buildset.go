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
	"fmt"
	"strconv"
	"strings"
)

// BuildSet is a parsed buildset tag value.
// It is implemented by *RietveldChange, *GerritChange, *GitilesCommit.
type BuildSet interface {
	fmt.Stringer

	isABuildSet()
}

// TagBuildSet is a key of a tag used to group related builds.
// See also ParseBuildSet.
// When a build triggers a new build, the buildset tag must be copied.
const TagBuildSet = "buildset"

// RietveldChange is a patchset on Rietveld.
type RietveldChange struct {
	Host     string
	Issue    int64
	PatchSet int
}

func (*RietveldChange) isABuildSet() {}

// String returns a buildset string for the change,
// e.g. "patch/rietveld/codereview.chromium.org/2979743003/1".
func (c *RietveldChange) String() string {
	return fmt.Sprintf("patch/rietveld/%s/%d/%d", c.Host, c.Issue, c.PatchSet)
}

// URL returns URL of the change.
func (c *RietveldChange) URL() string {
	return fmt.Sprintf("https://%s/%d/#ps%d", c.Host, c.Issue, c.PatchSet)
}

// GerritChange is a patchset on gerrit.
type GerritChange struct {
	Host     string
	Change   int64
	PatchSet int
}

func (*GerritChange) isABuildSet() {}

// String returns a buildset string for the change,
// e.g. "patch/gerrit/chromium-review.googlesource.com/677784/5".
func (c *GerritChange) String() string {
	return fmt.Sprintf("patch/gerrit/%s/%d/%d", c.Host, c.Change, c.PatchSet)
}

// URL returns URL of the change.
func (c *GerritChange) URL() string {
	return fmt.Sprintf("https://%s/c/%d/%d", c.Host, c.Change, c.PatchSet)
}

// GitilesCommit is a Git commit on a Gitiles server.
// Builds that have a Gitiles commit buildset, usually also have gitiles_ref
// tag.
type GitilesCommit struct {
	Host     string
	Project  string
	Revision string // i.e. commit hex sha1
}

func (*GitilesCommit) isABuildSet() {}

// String returns a buildset string for the commit,
// e.g. "commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7"
func (c *GitilesCommit) String() string {
	return fmt.Sprintf("commit/gitiles/%s/%s/+/%s", c.Host, c.Project, c.Revision)
}

// RepoURL returns the URL for the gitiles repo.
// e.g. "https://chromium.googlesource.com/chromium/src"
func (c *GitilesCommit) RepoURL() string {
	return fmt.Sprintf("https://%s/%s", c.Host, c.Project)
}

// URL returns the URL for the gitiles commit.
// e.g. "https://chromium.googlesource.com/chromium/src/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7"
func (c *GitilesCommit) URL() string {
	return fmt.Sprintf("%s/+/%s", c.RepoURL(), c.Revision)
}

// ParseBuildSet tries to parse buildset as one of the known formats.
// If buildSet was not recognized, returns nil.
func ParseBuildSet(buildSet string) BuildSet {
	// fmt.Sscanf cannot be used for this parsing because
	//   var a, b string
	//   fmt.Scanf("a/b", "%s/%s", &a, &b)
	//   a == "a/b", b == ""

	p := strings.Split(buildSet, "/")
	for _, c := range p {
		if c == "" {
			return nil
		}
	}
	n := len(p)
	switch {
	case n == 5 && p[0] == "patch" && p[1] == "gerrit":
		gerrit := &GerritChange{
			Host: p[2],
		}
		var err error
		if gerrit.Change, err = strconv.ParseInt(p[3], 10, 64); err != nil {
			return nil
		}
		if gerrit.PatchSet, err = strconv.Atoi(p[4]); err != nil {
			return nil
		}
		return gerrit

	case n == 5 && p[0] == "patch" && p[1] == "rietveld":
		rietveld := &RietveldChange{
			Host: p[2],
		}
		var err error
		if rietveld.Issue, err = strconv.ParseInt(p[3], 10, 64); err != nil {
			return nil
		}
		if rietveld.PatchSet, err = strconv.Atoi(p[4]); err != nil {
			return nil
		}
		return rietveld

	case n >= 5 && p[0] == "commit" && p[1] == "gitiles":
		if p[n-2] != "+" || !looksLikeSha1(p[n-1]) {
			return nil
		}
		return &GitilesCommit{
			Host:     p[2],
			Project:  strings.Join(p[3:n-2], "/"), // exclude plus
			Revision: p[n-1],
		}

	default:
		return nil
	}
}

func looksLikeSha1(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		switch {
		case '0' <= c && c <= '9':
		case 'a' <= c && c <= 'f':
		default:
			return false
		}
	}
	return true
}
