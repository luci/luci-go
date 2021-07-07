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

package prjpb

import (
	"fmt"
	"sort"

	"go.chromium.org/luci/cv/internal/changelist"
)

// TODO(tandrii): move this to changelist.
type sortableCLUpdated []*changelist.CLUpdatedEvent

func (s sortableCLUpdated) Len() int               { return len(s) }
func (s sortableCLUpdated) Less(i int, j int) bool { return s[i].GetClid() < s[j].GetClid() }
func (s sortableCLUpdated) Swap(i int, j int)      { s[i], s[j] = s[j], s[i] }

// MakeCLsUpdated returns CLsUpdated given the CLs.
//
// In each given CL, .ID and .EVersion must be set.
func MakeCLsUpdated(cls []*changelist.CL) *changelist.CLUpdatedEvents {
	events := make(sortableCLUpdated, len(cls))
	for i, cl := range cls {
		if cl.ID == 0 || cl.EVersion == 0 {
			panic(fmt.Errorf("ID %d and EVersion %d must not be 0", cl.ID, cl.EVersion))
		}
		events[i] = &changelist.CLUpdatedEvent{
			Clid:     int64(cl.ID),
			Eversion: int64(cl.EVersion),
		}
	}
	sort.Sort(events)
	return &changelist.CLUpdatedEvents{
		Events: events,
	}
}
