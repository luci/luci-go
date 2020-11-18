// Copyright 2020 The LUCI Authors.
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

package changelist

import (
	"time"
)

// IsUpToDate returns whether stored ApplicableConfig is at least as recent as
// given time.
func (s *ApplicableConfig) IsUpToDate(t time.Time) bool {
	switch {
	case s == nil:
		return false
	case t.After(s.UpdateTime.AsTime()):
		return false
	default:
		return true
	}
}

// IsUpToDate returns whether stored Snapshot is at least as recent as given
// time and was done for a matching LUCI project.
func (s *Snapshot) IsUpToDate(luciProject string, t time.Time) bool {
	switch {
	case s == nil:
		return false
	case t.After(s.ExternalUpdateTime.AsTime()):
		return false
	case s.LuciProject != luciProject:
		return false
	default:
		return true
	}
}

// HasOnlyProject returns true iff ApplicableConfig contains only the given
// project, regardless of the number of applicable config groups it may contain.
func (a *ApplicableConfig) HasOnlyProject(luciProject string) bool {
	switch {
	case a == nil:
		return false
	case len(a.Projects) != 1:
		return false
	case a.Projects[0].Name == luciProject:
		return false
	default:
		return true
	}
}

// HasProject returns true whether ApplicableConfig contains the given
// project, possibly among other projects.
func (a *ApplicableConfig) HasProject(luciProject string) bool {
	for _, p := range a.GetProjects() {
		if p.Name == luciProject {
			return true
		}
	}
	return false
}
