// Copyright 2023 The LUCI Authors.
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
	"context"
	"fmt"
	"sort"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

// stageTriggerCLDeps creates TriggeringCLsTasks(s) for CLs that shoul propagate
// its CQ votes to its deps.
func stageTriggerCLDeps(ctx context.Context, cls map[int64]*clInfo, pm pmState) []*prjpb.TriggeringCLDeps {
	if len(cls) == 0 {
		return nil
	}
	clsToTriggerDeps := make(map[int64]*clInfo, len(cls))
	clsShouldNotTriggerDeps := make(map[int64]struct{}, len(cls))
	for _, clid := range computeSortedCLIDs(cls) {
		info := cls[clid]

		switch {
		case info.deps == nil:
		case len(info.deps.needToTrigger) == 0:
		case canScheduleTriggerCLDeps(ctx, clid, cls):
			clsToTriggerDeps[clid] = info
		default:
			// If a clinfo reaches here,
			// - the CL has at least one dep to trigger
			// - however, canScheduleTriggerCLDeps() says don't schedule
			//   a new one. e.g., it already has TriggeringCLDeps op.
			//
			// If a CL shouldn't schedule a new one, none of its deps should
			// either.
			for _, dep := range info.pcl.GetDeps() {
				clsShouldNotTriggerDeps[dep.GetClid()] = struct{}{}
			}
		}
	}

	// schedule new tasks for the top most CLs only.
	for _, ci := range clsToTriggerDeps {
		if _, exist := clsShouldNotTriggerDeps[ci.pcl.GetClid()]; exist {
			delete(clsToTriggerDeps, ci.pcl.GetClid())
		}
		for _, dep := range ci.pcl.GetDeps() {
			switch _, ok := clsToTriggerDeps[dep.GetClid()]; {
			case !ok:
				continue
			default:
				delete(clsToTriggerDeps, dep.GetClid())
			}
		}
	}
	var ret []*prjpb.TriggeringCLDeps
	for clid, ci := range clsToTriggerDeps {
		t := &prjpb.TriggeringCLDeps{
			OriginClid:      clid,
			DepClids:        make([]int64, len(ci.deps.needToTrigger)),
			Trigger:         ci.pcl.GetTriggers().GetCqVoteTrigger(),
			ConfigGroupName: pm.ConfigGroup(ci.pcl.GetConfigGroupIndexes()[0]).ID.Name(),
		}
		for i, dep := range ci.deps.needToTrigger {
			t.DepClids[i] = dep.GetClid()
		}
		logging.Infof(ctx, "Scheduling a TriggeringCLDeps for clid %d with deps %v",
			t.GetOriginClid(), t.GetDepClids())
		ret = append(ret, t)
	}
	return ret
}

func computeSortedCLIDs(clinfos map[int64]*clInfo) []int64 {
	// sort cls by clid in descending order to produce a consistent decision
	// for OriginClid.
	if len(clinfos) == 0 {
		return nil
	}
	clids := make([]int64, 0, len(clinfos))
	for clid := range clinfos {
		clids = append(clids, clid)
	}
	sort.Slice(clids, func(i, j int) bool {
		return clids[i] > clids[j]
	})
	return clids
}

// canScheduleTriggerCLDeps returns whether triager can schedule
// a new TriggeringCLDeps for a given CL.
//
// Panic if the ci has no needToTrigger
func canScheduleTriggerCLDeps(ctx context.Context, clid int64, cls map[int64]*clInfo) bool {
	ci := cls[clid]
	if ci == nil || ci.deps == nil || len(ci.deps.needToTrigger) == 0 {
		panic(fmt.Errorf("canScheduleTriggerCLDeps called with 0 needToTrigger"))
	}
	cqMode := run.Mode(ci.pcl.GetTriggers().GetCqVoteTrigger().GetMode())
	if cqMode == "" {
		return false
	}
	switch {
	case ci.pcl.GetOutdated() != nil:
		return false
	case len(ci.deps.notYetLoaded) > 0:
		return false
	case ci.purgingCL != nil || len(ci.purgeReasons) > 0:
		return false
	case ci.triggeringCLDeps != nil:
		return false
	case ci.hasIncompleteRun(cqMode):
		return false
	}
	// If the dep is currently being purged or triggered, don't schedule
	// a new task, until they are done. For example, given CL{1-5} with CL1
	// being the bottommost CL, let's say that
	// - the CL author triggers CQ+2 on CL3.
	// - while the TriggeringCLDeps{} for CL{1,2,3} is in progress,
	//   the CL author triggers CQ+2 on CL5.
	//
	// If so, wait until the TriggeringCLDeps{} for CL{1,2,3} is done
	// to remove unusual corner cases.
	for _, dep := range ci.pcl.GetDeps() {
		switch dci, ok := cls[dep.GetClid()]; {
		case !ok:
			continue
		case dci.pcl.GetOutdated() != nil:
			return false
		case dci.purgingCL != nil, len(dci.purgeReasons) > 0:
			return false
		case dci.triggeringCLDeps != nil:
			return false
		case dci.hasIncompleteRun(cqMode) && (dci.deps != nil && len(dci.deps.needToTrigger) > 0):
			// This is the case where
			// - a dep CL has an ongoing Run,
			// - the dep CL has dependenies, and
			// - at least one of the deps of the dep CL have no CQ+2 votes.
			//
			// Let's say that there are CL{1-3}, with CL1 being the bottommost
			// CL and the following conditions.
			// - CL1 with CQ(0).
			// - CL2 with CQ(+2) and an ongoing run.
			// - CL3 with CQ(+2) and no run.
			//
			// This can happen only if the Run for CL1 failed, but CL2 hasn't
			// ended yet. PM should NOT schedule a new TriggeringCLDeps for CL3,
			// until CL2 ends.
			return false
		}
	}
	return true
}
