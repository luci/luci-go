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
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// stagePurges sets .purgeCLtasks and returns `now` if any CLs can be purged.
//
// Otherwise, returns the time of the earliest purge or Zero time if no purges
// should be done.
func (a *actor) stagePurges(ctx context.Context, now time.Time) time.Time {
	earliest := time.Time{}
	for clid, info := range a.cls {
		switch when := a.purgeETA(info, now); {
		case when.IsZero():
		case when.After(now):
			if earliest.IsZero() || earliest.After(when) {
				earliest = when
			}
		default:
			earliest = now
			a.purgeCLtasks = append(a.purgeCLtasks, &prjpb.PurgeCLTask{
				Reasons:   a.cls[clid].purgeReasons,
				PurgingCl: &prjpb.PurgingCL{Clid: clid},
			})
		}
	}
	return earliest
}

// purgeETA returns the earliest time a CL may be purged and Zero time if CL
// should not be purged at all.
func (a *actor) purgeETA(info *clInfo, now time.Time) time.Time {
	if info.purgeReasons == nil {
		return time.Time{}
	}
	if len(info.pcl.GetConfigGroupIndexes()) != 1 {
		// In case of bad config, waiting doesn't help.
		return now
	}
	cg := a.s.ConfigGroup(info.pcl.GetConfigGroupIndexes()[0])
	d := cg.Content.GetCombineCls().GetStabilizationDelay()
	if d == nil {
		return now
	}
	t := info.lastTriggered()
	if t.IsZero() {
		panic(fmt.Errorf("CL %d which is not triggered can't be purged (reasons: %s)", info.pcl.GetClid(), info.purgeReasons))
	}
	when := t.Add(d.AsDuration())
	if when.After(now) {
		return when
	}
	return now
}
