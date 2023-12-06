// Copyright 2022 The LUCI Authors.
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

package run

import (
	"fmt"
)

// HasTriggerChanged checks whether a trigger is no longer valid for a given CL.
//
// A trigger is no longer valid if it is no longer found in the CL.
func HasTriggerChanged(old *Trigger, ts *Triggers, clURL string) string {
	cur := ts.GetCqVoteTrigger()
	if Mode(old.Mode) == NewPatchsetRun {
		cur = ts.GetNewPatchsetRunTrigger()
	}

	switch {
	case cur == nil:
		return fmt.Sprintf("the %s trigger on %s has been removed", old.GetMode(), clURL)
	case old.GetMode() != cur.GetMode():
		return fmt.Sprintf("the triggering vote on %s has requested a different run mode: %s", clURL, cur.GetMode())
	case !old.GetTime().AsTime().Equal(cur.GetTime().AsTime()):
		return fmt.Sprintf(
			"the timestamp of the triggering vote on %s has changed from %s to %s",
			clURL, old.GetTime().AsTime(), cur.GetTime().AsTime())
	default:
		// Theoretically, if the triggering user changes, it should also be counted
		// as changed trigger. However, checking whether triggering time has changed
		// is ~enough because it is extremely rare that votes from different users
		// have the exact same timestamp. And even if it really happens, CV won't
		// be able to handle this case because CV will generate the same Run ID as
		// user is not taken into account during ID generation.
		return ""
	}
}

// WithTrigger assigns the given Trigger to the correct slot of the Triggers
// struct depending on the trigger mode. I.e. distinguish CQ Vote triggers from
// New Patchset triggers.
//
// Returns a reference to the modified/instantiated Triggers.
func (ts *Triggers) WithTrigger(t *Trigger) *Triggers {
	if ts == nil {
		ts = &Triggers{}
	}
	if Mode(t.Mode) == NewPatchsetRun {
		ts.NewPatchsetRunTrigger = t
	} else {
		ts.CqVoteTrigger = t
	}
	return ts
}
