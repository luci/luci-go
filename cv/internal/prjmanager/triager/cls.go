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

package triager

import (
	"fmt"
	"time"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// triageCLs decides whether individual CLs ought to be acted upon.
func triageCLs(c *prjpb.Component, pm pmState) map[int64]*clInfo {
	cls := make(map[int64]*clInfo, len(c.GetClids()))
	for _, clid := range c.GetClids() {
		cls[clid] = &clInfo{
			pcl:       pm.MustPCL(clid),
			purgingCL: pm.PurgingCL(clid), // may be nil
		}
	}
	for index, r := range c.GetPruns() {
		for _, clid := range r.GetClids() {
			info := cls[clid]
			info.runIndexes = append(info.runIndexes, int32(index))
		}
	}
	for _, info := range cls {
		info.triage(pm)
	}
	return cls
}

// clInfo represents a CL in the PM component of CLs.
type clInfo struct {
	pcl *prjpb.PCL
	// runIndexes are indexes of Component.PRuns which references this CL.
	runIndexes []int32
	// purgingCL is set if CL is already being purged.
	purgingCL *prjpb.PurgingCL

	triagedCL
}

// lastTriggered returns the last triggered time among this CL and its triggered
// deps. Can be zero time.Time if neither are triggered.
func (c *clInfo) lastTriggered() time.Time {
	thisPB := c.pcl.GetTrigger().GetTime()
	switch {
	case thisPB == nil && c.deps == nil:
		return time.Time{}
	case thisPB == nil:
		return c.deps.lastTriggered
	case c.deps == nil || c.deps.lastTriggered.IsZero():
		return thisPB.AsTime()
	default:
		this := thisPB.AsTime()
		if c.deps.lastTriggered.Before(this) {
			return this
		}
		return c.deps.lastTriggered
	}
}

// triagedCL is the result of CL triage (see clInfo.triage()).
//
// Note: This doesn't take into account `combine_cls.stabilization_delay`,
// thus a CL may be ready or with purgeReason, but due to stabilization delay,
// it shouldn't be acted upon *yet*.
type triagedCL struct {
	// deps are triaged deps, set only if CL is watched by exactly 1 config group.
	// of the current project.
	deps *triagedDeps
	// purgeReasons is set if the CL ought to be purged.
	//
	// Not set if CL is .purgingCL is non-nil since CL is already being purged.
	purgeReasons []*changelist.CLError
	// ready is true if it can be used in creation of new Runs.
	//
	// If true, purgeReason must be nil, and deps must be OK though they may contain
	// not-yet-loaded deps.
	ready bool
}

// triage sets the triagedCL part of clInfo.
//
// Expects non-triagedCL part of clInfo to be already set.
// panics iff component is not in a valid state.
func (info *clInfo) triage(pm pmState) {
	switch {
	case len(info.runIndexes) > 0:
		// Once CV supports API-based triggering, a CL may be both in purged
		// state and have an incomplete Run at the same time. The presence in a
		// Run is more important, so treat it as such.
		info.triageInRun(pm)
	case info.purgingCL != nil:
		info.triageInPurge(pm)
	default:
		info.triageNew(pm)
	}
}

func (info *clInfo) triageInRun(pm pmState) {
	pcl := info.pcl
	switch s := pcl.GetStatus(); {
	case (false || // false is a noop for ease of reading
		s == prjpb.PCL_DELETED ||
		s == prjpb.PCL_UNWATCHED ||
		s == prjpb.PCL_UNKNOWN ||
		pcl.GetSubmitted() ||
		pcl.GetTrigger() == nil):
		// This is expected while Run is being finalized iff PM sees updates to a
		// CL before OnRunFinished event.
		return
	case len(pcl.GetConfigGroupIndexes()) != 1:
		// This is expected if project config has changed, but Run's reaction to it via
		// OnRunFinished event hasn't yet reached PM.
		return
	case s != prjpb.PCL_OK:
		panic(fmt.Errorf("PCL has unrecognized status %s", s))
	}

	cgIndex := pcl.GetConfigGroupIndexes()[0]
	info.deps = triageDeps(pcl, cgIndex, pm)
	// A purging CL must not be "ready" to avoid creating new Runs with them.
	if info.deps.OK() && info.purgingCL == nil {
		info.ready = true
	}
}

func (info *clInfo) triageInPurge(pm pmState) {
	// The PM hasn't noticed yet the completion of the async purge.
	// The result of purging is modified CL, which may be observed by PM earlier
	// than completion of purge.
	//
	// Thus, consider these CLs in potential Run Creation, but don't mark them
	// ready in order to avoid creating new Runs.
	pcl := info.pcl
	switch s := pcl.GetStatus(); s {
	case prjpb.PCL_DELETED, prjpb.PCL_UNWATCHED, prjpb.PCL_UNKNOWN:
		// Likely purging effect is already visible.
		return
	case prjpb.PCL_OK:
		// OK.
	default:
		panic(fmt.Errorf("PCL has unrecognized status %s", s))
	}

	switch {
	case pcl.GetSubmitted():
		// Someone already submitted CL while purging was ongoing.
		return
	case pcl.GetTrigger() == nil:
		// Likely purging effect is already visible.
		return
	}

	cgIndexes := pcl.GetConfigGroupIndexes()
	switch len(cgIndexes) {
	case 0:
		panic(fmt.Errorf("PCL %d without ConfigGroup index not possible for CL not referenced by any Runs (partitioning bug?)", pcl.GetClid()))
	case 1:
		info.deps = triageDeps(pcl, cgIndexes[0], pm)
		// info.deps.OK() may be true, for example if user has already corrected the
		// mistake that previously reulted in purging op. However, don't mark CL
		// ready until purging op completes or expires.
	}
}

func (info *clInfo) triageNew(pm pmState) {
	pcl := info.pcl
	clid := pcl.GetClid()
	assumption := "not possible for CL not referenced by any Runs (partitioning bug?)"
	switch s := pcl.GetStatus(); s {
	case prjpb.PCL_DELETED, prjpb.PCL_UNWATCHED, prjpb.PCL_UNKNOWN:
		panic(fmt.Errorf("PCL %d status %s %s", clid, s, assumption))
	case prjpb.PCL_OK:
		// OK.
	default:
		panic(fmt.Errorf("PCL has unrecognized status %s", s))
	}

	switch {
	case pcl.GetSubmitted():
		panic(fmt.Errorf("PCL %d submitted %s", clid, assumption))
	case pcl.GetTrigger() == nil:
		panic(fmt.Errorf("PCL %d not triggered %s", clid, assumption))
	case len(pcl.GetErrors()) > 0:
		info.purgeReasons = pcl.GetErrors()
		return
	}

	cgIndexes := pcl.GetConfigGroupIndexes()
	switch len(cgIndexes) {
	case 0:
		panic(fmt.Errorf("PCL %d without ConfigGroup index %s", clid, assumption))
	case 1:
		if info.deps = triageDeps(pcl, cgIndexes[0], pm); info.deps.OK() {
			info.ready = true
		} else {
			info.purgeReasons = append(info.purgeReasons, info.deps.makePurgeReason())
		}
	default:
		cgNames := make([]string, len(cgIndexes))
		for i, idx := range cgIndexes {
			cgNames[i] = pm.ConfigGroup(idx).ID.Name()
		}
		info.purgeReasons = append(info.purgeReasons, &changelist.CLError{
			Kind: &changelist.CLError_WatchedByManyConfigGroups_{
				WatchedByManyConfigGroups: &changelist.CLError_WatchedByManyConfigGroups{
					ConfigGroups: cgNames,
				},
			},
		})
	}
}
