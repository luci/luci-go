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

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
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

// SemanticallyEqual checks if ApplicableConfig configs are the same except the
// update time.
func (a *ApplicableConfig) SemanticallyEqual(b *ApplicableConfig) bool {
	if len(a.GetProjects()) != len(b.GetProjects()) {
		return false
	}
	for i, pa := range a.GetProjects() {
		switch pb := b.GetProjects()[i]; {
		case pa.GetName() != pb.GetName():
			return false
		case len(pa.GetConfigGroupIds()) != len(pb.GetConfigGroupIds()):
			return false
		default:
			for j, sa := range pa.GetConfigGroupIds() {
				if sa != pb.GetConfigGroupIds()[j] {
					return false
				}
			}
		}
	}
	return true
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

// PanicIfNotValid checks that Snapshot stored has required fields set.
func (s *Snapshot) PanicIfNotValid() {
	switch {
	case s == nil:
	case s.GetExternalUpdateTime() == nil:
		panic("missing ExternalUpdateTime")
	case s.GetLuciProject() == "":
		panic("missing LuciProject")
	case s.GetMinEquivalentPatchset() == 0:
		panic("missing MinEquivalentPatchset")
	case s.GetPatchset() == 0:
		panic("missing Patchset")

	case s.GetGerrit() == nil:
		panic("Gerrit is required, until CV supports more code reviews")
	case s.GetGerrit().GetInfo() == nil:
		panic("Gerrit.Info is required, until CV supports more code reviews")
	}
}

// RemoveUnusedGerritInfo mutates given ChangeInfo to remove what CV definitely
// doesn't need to reduce bytes shuffled to/from Datastore.
//
// Doesn't complain if anything is missing.
//
// NOTE: keep this function actions in sync with storage.proto doc for
// Gerrit.info field.
func RemoveUnusedGerritInfo(ci *gerritpb.ChangeInfo) {
	cleanUser := func(u *gerritpb.AccountInfo) {
		if u == nil {
			return
		}
		u.SecondaryEmails = nil
		u.Name = ""
		u.Username = ""
	}

	cleanRevision := func(r *gerritpb.RevisionInfo) {
		if r == nil {
			return
		}
		if c := r.GetCommit(); c != nil {
			c.Parents = nil
		}
		r.Uploader = nil
		r.Files = nil
	}

	cleanMessage := func(m *gerritpb.ChangeMessageInfo) {
		if m == nil {
			return
		}
		cleanUser(m.GetAuthor())
		cleanUser(m.GetRealAuthor())
	}

	cleanLabel := func(l *gerritpb.LabelInfo) {
		if l == nil {
			return
		}
		for _, a := range l.GetAll() {
			cleanUser(a.GetUser())
		}
	}
	for _, r := range ci.GetRevisions() {
		cleanRevision(r)
	}
	for _, m := range ci.GetMessages() {
		cleanMessage(m)
	}
	for _, l := range ci.GetLabels() {
		cleanLabel(l)
	}
	cleanUser(ci.GetOwner())
}
