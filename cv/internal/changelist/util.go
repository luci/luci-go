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
	"context"
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
	case t.After(s.GetExternalUpdateTime().AsTime()):
		return false
	case s.GetLuciProject() != luciProject:
		return false
	default:
		return true
	}
}

// HasOnlyProject returns true iff ApplicableConfig contains only the given
// project, regardless of the number of applicable config groups it may contain.
func (a *ApplicableConfig) HasOnlyProject(luciProject string) bool {
	projects := a.GetProjects()
	if len(projects) != 1 {
		return false
	}
	return projects[0].GetName() == luciProject
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

// NeedsFetching returns true if the CL is likely out of date and would benefit
// from fetching in the context of a given project.
func (cl *CL) NeedsFetching(ctx context.Context, luciProject string) (bool, error) {
	switch {
	case cl == nil:
		panic("CL must be not nil")
	case cl.Snapshot == nil:
		return true, nil
	case cl.Snapshot.GetLuciProject() != luciProject:
		// TODO(tandrii): verify here which luciProject is allowed to watch the repo.
		return true, nil
	default:
		// TODO(tandrii): add mechanism to refresh purely due to passage of time.
		return false, nil
	}
}
