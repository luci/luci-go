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
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

// stagePurges returns either purgeCLtasks for immediate purging OR the earliest
// time when a CL should be purged. Zero time means no purges to be done.
func stagePurges(ctx context.Context, cls map[int64]*clInfo, pm pmState) ([]*prjpb.PurgeCLTask, time.Time) {
	now := clock.Now(ctx)
	var out []*prjpb.PurgeCLTask
	next := time.Time{}
	for clid, info := range cls {
		if info.purgingCL != nil || info.pcl.GetOutdated() != nil {
			// the CL is already being purged, do not schedule a new task.
			continue
		}
		switch when := purgeETA(info, now, pm); {
		case when.IsZero():
		case when.After(now):
			next = earliest(next, when)
		default:
			purgingCl := &prjpb.PurgingCL{
				Clid: clid,
			}
			var triggers *run.Triggers
		loop:
			for _, pr := range info.purgeReasons {
				switch v := pr.ApplyTo.(type) {
				case *prjpb.PurgeReason_AllActiveTriggers:
					purgingCl.ApplyTo = &prjpb.PurgingCL_AllActiveTriggers{AllActiveTriggers: true}
					break loop
				case *prjpb.PurgeReason_Triggers:
					if triggers == nil {
						triggers = &run.Triggers{}
						purgingCl.ApplyTo = &prjpb.PurgingCL_Triggers{Triggers: triggers}
					}
					proto.Merge(triggers, v.Triggers)
				}
			}
			out = append(out, &prjpb.PurgeCLTask{
				PurgeReasons: info.purgeReasons,
				PurgingCl:    purgingCl,
			})
		}
	}
	return out, next
}
func (info *clInfo) isTriggered() bool {
	return info.pcl.GetTriggers().GetCqVoteTrigger() != nil || info.pcl.GetTriggers().GetNewPatchsetRunTrigger() != nil
}

// purgeETA returns the earliest time a CL may be purged and Zero time if CL
// should not be purged at all.
func purgeETA(info *clInfo, now time.Time, pm pmState) time.Time {
	if info.purgeReasons == nil {
		return time.Time{}
	}
	if len(info.pcl.GetConfigGroupIndexes()) != 1 {
		// In case of bad config, waiting doesn't help.
		return now
	}
	if !info.isTriggered() {
		panic(fmt.Errorf("CL %d is not triggered and thus can't be purged (reasons: %s)", info.pcl.GetClid(), info.purgeReasons))
	}
	var eta time.Time
	if info.pcl.GetTriggers().GetNewPatchsetRunTrigger() != nil {
		eta = earliest(eta, now)
	}
	if info.pcl.GetTriggers().GetCqVoteTrigger() != nil {
		cg := pm.ConfigGroup(info.pcl.GetConfigGroupIndexes()[0])
		d := cg.Content.GetCombineCls().GetStabilizationDelay()
		if d == nil {
			eta = earliest(eta, now)
		} else {

			t := info.lastCQVoteTriggered()
			if t.IsZero() {
				panic(fmt.Errorf("impossible: CQ label is triggered but triggering time is zero"))
			}
			eta = earliest(eta, t.Add(d.AsDuration()))
		}
	}
	return eta
}
