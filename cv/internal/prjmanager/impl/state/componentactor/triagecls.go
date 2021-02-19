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

package componentactor

import (
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// triageCLs computes and sets .cls and .reverseDeps.
func (a *Actor) triageCLs() {
	a.cls = make(map[int64]*clInfo, len(a.c.GetClids()))
	for _, clid := range a.c.GetClids() {
		a.cls[clid] = &clInfo{
			pcl:       a.s.MustPCL(clid),
			purgingCL: a.s.PurgingCL(clid), // may be nil
		}
	}
	for index, r := range a.c.GetPruns() {
		for _, clid := range r.GetClids() {
			t := a.cls[clid]
			t.runIndexes = append(t.runIndexes, int32(index))
		}
	}

	a.reverseDeps = map[int64][]int64{}
	for clid, info := range a.cls {
		a.triageCL(clid, info)
		switch {
		case info.ready:
			info.deps.iterateNotSubmitted(info.pcl, func(dep *changelist.Dep) {
				did := dep.GetClid()
				a.reverseDeps[did] = append(a.reverseDeps[did], clid)
			})
		case info.purgeReason != nil:
			a.toPurge[clid] = struct{}{}
		}
	}
}

// clInfo represents a CL in the Actor's component.
type clInfo struct {
	pcl *prjpb.PCL

	// runIndexes are indexes of Component.PRuns which references this CL.
	runIndexes []int32
	// purgingCL is set if CL is already being purged.
	purgingCL *prjpb.PurgingCL

	// deps are triaged deps, set only if CL is watched by exactly 1 config group.
	// of the current project.
	deps *triagedDeps
	// purgeReason is set with details for purge reason if the CL ought to be purged.
	//
	// Not set is CL is .purgingCL is non-nil since CL is already being purged.
	purgeReason *prjpb.PurgeCLTask_Reason
	// ready is true if it can be used in creation of new Runs.
	//
	// If true, purgeReason must be nil, and deps must be OK though they may contain
	// not not yet loaded deps.
	ready bool
}

// triageCL updates clInfo expecting runIndexes and purginCL to be already set.
//
// panics iff component is not in a valid state.
func (a *Actor) triageCL(clid int64, info *clInfo) {
	switch {
	case len(info.runIndexes) > 0:
		// Once CV supports API based triggering, a CL may be at the same time be in
		// purged state && have an incomplete Run. The presence in a Run is more
		// important, so treat as such.
		a.triageCLInRun(clid, info)
	case info.purgingCL != nil:
		a.triageCLInPurge(clid, info)
	default:
		a.triageCLNew(clid, info)
	}
}

func (a *Actor) triageCLInRun(clid int64, info *clInfo) {
	// TODO: implement
}

func (a *Actor) triageCLInPurge(clid int64, info *clInfo) {
	// TODO: implement
}

func (a *Actor) triageCLNew(clid int64, info *clInfo) {
	// TODO: implement
}
