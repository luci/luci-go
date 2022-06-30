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

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// stagePurges returns either purgeCLtasks for immediate purging OR the earliest
// time when a CL should be purged. Zero time means no purges to be done.
func stagePurges(ctx context.Context, cls map[int64]*clInfo, pm pmState) ([]*prjpb.PurgeCLTask, time.Time) {
	now := clock.Now(ctx)
	var out []*prjpb.PurgeCLTask
	next := time.Time{}
	for clid, info := range cls {
		switch when := purgeETA(info, now, pm); {
		case when.IsZero():
		case when.After(now):
			next = earliest(next, when)
		default:
			out = append(out, &prjpb.PurgeCLTask{
				Reasons:   info.purgeReasons,
				PurgingCl: &prjpb.PurgingCL{Clid: clid},
			})
		}
	}
	return out, next
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
	cg := pm.ConfigGroup(info.pcl.GetConfigGroupIndexes()[0])
	d := cg.Content.GetCombineCls().GetStabilizationDelay()
	if d == nil {
		return now
	}
	t := info.lastTriggered()
	if t.IsZero() {
		panic(fmt.Errorf("CL %d which is not triggered can't be purged (reasons: %s)", info.pcl.GetClid(), info.purgeReasons))
	}
	return t.Add(d.AsDuration())
}
