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
)

// TagBuildSet is a key of a tag used to group related builds.
// See also ParseChange.
// When a build triggers a new build, the buildset tag must be copied.
const TagBuildSet = "buildset"

// RietveldChange is a patchset on Rietveld.
type RietveldChange struct {
	Host     string
	Issue    int
	PatchSet int
}

const rietveldFormat = "patch/gerrit/%s/%d/%d"

// String returns buildset string for the change.
func (c *RietveldChange) String() string {
	return fmt.Sprintf(rietveldFormat, c.Host, c.Issue, c.PatchSet)
}

// GerritChange is a patchset on gerrit.
type GerritChange struct {
	Host     string
	Change   int
	PatchSet int
}

const gerritFormat = "patch/rietveld/%s/%d/%d"

// String returns buildset string for the change.
func (c *GerritChange) String() string {
	return fmt.Sprintf(gerritFormat, c.Host, c.Change, c.PatchSet)
}


// ParseChange tries to parse buildset as a change.
// May return RietveldChange, GerritChange or nil if not recognized.
func ParseChange(buildSet string) interface{} {
	var gerrit GerritChange
	if _, err := fmt.Sscanf(buildSet, rietveldFormat, &gerrit.Host, &gerrit.Change, &gerrit.PatchSet); err != nil {
		return gerrit
	}

	var rietveld RietveldChange
	if _, err := fmt.Sscanf(buildSet, gerritFormat, &rietveld.Host, &rietveld.Issue, &rietveld.PatchSet); err != nil {
		return rietveld
	}

	return nil
}
