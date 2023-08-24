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

	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// stageTriggerDeps creates TriggeringCLsTasks(s) for the deps that should be
// triggered.
func stageTriggerDeps(ctx context.Context, cls map[int64]*clInfo, pm pmState) ([]*prjpb.TriggeringCL, error) {
	var depsToTrigger []*prjpb.TriggeringCL
	depsAdded := make(map[int64]struct{})

	for _, info := range cls {
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
			// sets the clinfo.triggeringCL so that it will be tasked only once.
			cls[dep.GetClid()].triggeringCL = tcl
			depsToTrigger = append(depsToTrigger, tcl)
			depsAdded[dep.GetClid()] = struct{}{}
		}
	}
	if len(depsToTrigger) == 0 {
		return nil, nil
	}
	// A dep in needToTrigger is a CL that is known to and tracked in
	// the Component.
	// i.e., it has an entry in `cls`.
	// Also, there is at least one CL that has a CQ vote and has the the dep
	// in its depenencies.
	//
	// In other words, all the deps in depsToTrigger are within the same
	// component and connected with each other either directly or indirectly.
	return depsToTrigger, nil
}
