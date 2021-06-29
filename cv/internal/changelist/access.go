// Copyright 2021 The LUCI Authors.
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
	"fmt"

	"go.chromium.org/luci/common/clock"
)

// AccessKind is the level of access a LUCI project has to a CL.
type AccessKind int

const (
	// AccessUnknown means a CL needs refreshing in the context of this project
	// in order to ascertain the AccessKind.
	AccessUnknown AccessKind = iota
	// AccessGranted means this LUCI project has exclusive access to the CL.
	//
	//  * this LUCI project is configured to watch this config,
	//    * and no other project is;
	//  * this LUCI project has access to the CL in code review (e.g., Gerrit);
	AccessGranted
	// AccessDeniedProbably means there is early evidence that LUCI project lacks
	// access to the project.
	//
	// This is a mitigation to Gerrit eventual consistency, which may result in
	// HTTP 404 returned for a CL that has just been created.
	AccessDeniedProbably
	// AccessDenied means the LUCI project has no access to this CL.
	//
	// Can be either due to project config not being the only watcher of the CL,
	// or due to the inability to fetch CL from code review (e.g. Gerrit).
	AccessDenied
)

// AccessKind returns AccessKind of a CL.
func (cl *CL) AccessKind(ctx context.Context, luciProject string) AccessKind {
	kind, _ := cl.AccessKindWithReason(ctx, luciProject)
	return kind
}

// AccessKind returns AccessKind of a CL from code review site.
func (cl *CL) AccessKindFromCodeReviewSite(ctx context.Context, luciProject string) AccessKind {
	if pa := cl.Access.GetByProject()[luciProject]; pa != nil {
		switch ct, now := pa.GetNoAccessTime(), clock.Now(ctx); {
		case ct == nil && pa.GetNoAccess():
			// Legacy not yet upgraded entity.
			return AccessDenied
		case ct == nil:
			panic(fmt.Errorf("Access.Project %q without NoAccess fields: %s", luciProject, pa))
		case now.Before(ct.AsTime()):
			return AccessDeniedProbably
		default:
			return AccessDenied
		}
	}
	return AccessGranted
}

// AccessKindWithReason returns AccessKind of a CL and a reason for it.
func (cl *CL) AccessKindWithReason(ctx context.Context, luciProject string) (AccessKind, string) {
	switch projects := cl.ApplicableConfig.GetProjects(); {
	case cl.ApplicableConfig == nil:
		// ApplicableConfig may not be always computable w/o first fetching CL from
		// code review, so this case is handled below.
	case len(projects) == 0:
		return AccessDenied, "not watched by any project"
	case len(projects) > 1:
		return AccessDenied, "watched not only by this project"
	case projects[0].GetName() != luciProject:
		return AccessDenied, "not watched by this project"
	default:
		// CL is watched by this project only.
	}

	switch cl.AccessKindFromCodeReviewSite(ctx, luciProject) {
	case AccessDenied:
		return AccessDenied, "code review site denied access"
	case AccessDeniedProbably:
		return AccessDeniedProbably, "code review site denied access recently"
	}

	if cl.ApplicableConfig == nil || cl.Snapshot == nil {
		return AccessUnknown, "needs a fetch from code review"
	}
	if cl.Snapshot.GetLuciProject() != luciProject {
		return AccessUnknown, "needs a fetch from code review due to Snapshot from old project"
	}
	return AccessGranted, "granted"
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

// SemanticallyEqual checks if ApplicableConfig configs are the same.
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
