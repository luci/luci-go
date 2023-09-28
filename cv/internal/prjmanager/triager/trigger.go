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
	"sort"

	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// stageTriggerDeps creates TriggeringCLsTasks(s) for the deps that should be
// triggered.
func stageTriggerDeps(ctx context.Context, cls map[int64]*clInfo, pm pmState) ([]*prjpb.TriggeringCL, error) {
	if len(cls) == 0 {
		return nil, nil
	}
	var depsToTrigger []*prjpb.TriggeringCL
	depsAdded := make(map[int64]struct{})
	for _, clid := range computeSortedCLIDs(cls) {
		info := cls[clid]
		if info.deps == nil {
			continue
		}
		// this CL is a dep of another CL, and is being triggered.
		if info.triggeringCL != nil {
			continue
		}
		// skip, if no trigger.
		cqTrigger := info.pcl.GetTriggers().GetCqVoteTrigger()
		if cqTrigger == nil {
			continue
		}

		for _, dep := range info.deps.needToTrigger {
			if _, ok := depsAdded[dep.GetClid()]; ok {
				continue
			}
			tcl := &prjpb.TriggeringCL{
				Clid:       dep.GetClid(),
				OriginClid: info.pcl.GetClid(),
				Trigger:    cqTrigger,
			}
			depsToTrigger = append(depsToTrigger, tcl)
			depsAdded[dep.GetClid()] = struct{}{}
		}
	}
	if len(depsToTrigger) == 0 {
		return nil, nil
	}
	return depsToTrigger, nil
}

func computeSortedCLIDs(clinfos map[int64]*clInfo) []int64 {
	// sort cls by clid in descending order to produce a consistent decision
	// for OriginClid.
	if len(clinfos) == 0 {
		return nil
	}
	clids := make([]int64, 0, len(clinfos))
	for clid, _ := range clinfos {
		clids = append(clids, clid)
	}
	sort.Slice(clids, func(i, j int) bool {
		return clids[i] > clids[j]
	})
	return clids
}
