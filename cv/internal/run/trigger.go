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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
)

// Triggers is a convenience type to represent the collection of triggers that
// can be present in a CL.
type Triggers []*Trigger

// cqVoteTriggers represent the set of modes that are triggered by a vote on
// the `Commit-Queue` Gerrit label.
var cqVoteTriggers = stringset.NewFromSlice(string(FullRun), string(DryRun), string(QuickDryRun))

// CQVoteTrigger returns the first trigger in the collection that is triggered
// by a vote on the `Commit-Queue` Gerrit label.
func (ts Triggers) CQVoteTrigger() *Trigger {
	return ts.firstInModes(cqVoteTriggers)
}

// Len returns the number of triggers in the collection.
func (ts Triggers) Len() int {
	return len(ts)
}

// firstInModes finds one trigger in the collection that matches any of the
// given set of modes.
func (ts Triggers) firstInModes(ms stringset.Set) *Trigger {
	for _, t := range ts {
		if ms.Has(t.Mode) {
			return t
		}
	}
	return nil
}

// Has checks whether the collection has the given trigger.
func (ts Triggers) Has(target *Trigger) bool {
	for _, t := range ts {
		if proto.Equal(t, target) {
			return true
		}
	}
	return false
}

// HasTriggerChanged checks whether a trigger is no longer valid for a given CL.
//
// A trigger is no longer valid if it is no longer present in the list of
// triggers found in the CL.
func HasTriggerChanged(old *Trigger, triggers Triggers, clURL string) string {
	switch t := triggers.firstInModes(stringset.NewFromSlice(old.GetMode())); {
	case triggers.Len() == 0 || t == nil:
		return fmt.Sprintf("the %s trigger on %s has been removed", old.GetMode(), clURL)
	case !old.GetTime().AsTime().Equal(t.GetTime().AsTime()):
		return fmt.Sprintf(
			"the timestamp of the triggering vote on %s has changed from %s to %s",
			clURL, old.GetTime().AsTime(), t.GetTime().AsTime())
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
