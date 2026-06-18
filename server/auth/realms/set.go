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

package realms

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
)

// Set is a set of global realm names.
type Set struct {
	// If true, all realms across all projects are in the set.
	all bool

	// Keys are project names, values are:
	//  * missing - no realm of this project are in the set.
	//  * nil - all realms of this project are in the set.
	//  * non-nil - a subset of realms of this project are in the set.
	byProject map[string]stringset.Set
}

// NewSet constructs a realm set from a list of entries.
//
// Each entry is either a concrete realm name "project:realm", a project
// wildcard "project:*" (matching any possible realm in this project) or
// a global wildcard "*" (matching absolutely any realm).
func NewSet(entries []string) (*Set, error) {
	set := &Set{}

	for idx, entry := range entries {
		switch {
		case entry == "*":
			set.all = true
			set.byProject = nil

		case strings.HasSuffix(entry, ":*"):
			project, _ := strings.CutSuffix(entry, ":*")
			if err := ValidateProjectName(project); err != nil {
				return nil, fmt.Errorf("entry #%d: %w", idx+1, err)
			}
			if set.all {
				continue
			}
			if set.byProject == nil {
				set.byProject = make(map[string]stringset.Set, 1)
			}
			set.byProject[project] = nil // means "all in the project"

		default:
			if err := ValidateRealmName(entry, GlobalScope); err != nil {
				return nil, fmt.Errorf("entry #%d: %w", idx+1, err)
			}
			if set.all {
				continue
			}
			project, realm := Split(entry)
			if set.byProject == nil {
				set.byProject = make(map[string]stringset.Set, 1)
			}
			switch perProj, ok := set.byProject[project]; {
			case !ok:
				// First time seeing this project.
				set.byProject[project] = stringset.NewFromSlice(realm)
			case perProj != nil:
				// Already have some realms from this project. Add one more.
				perProj.Add(realm)
			default:
				// All project realms are already included. No need to add more.
			}
		}
	}

	return set, nil
}

// Has returns true if the given full realm is in the set.
//
// Panics if this is not a valid full realm name. If this is a concern, use
// ValidateRealmName explicitly.
func (s *Set) Has(realm string) bool {
	// Note: do the split even if s.all is true since we promised to panic on
	// invalid realm name (Split does that).
	project, realm := Split(realm)
	if s.all {
		return true
	}
	perProj, ok := s.byProject[project]
	return ok && (perProj == nil || perProj.Has(realm))
}
