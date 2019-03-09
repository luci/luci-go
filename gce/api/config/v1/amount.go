// Copyright 2019 The LUCI Authors.
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

package config

import (
	"sort"
	"time"

	"go.chromium.org/luci/config/validation"
)

// indexedSchedule encapsulates a *Schedule and its original index in the
// *Amount.
type indexedSchedule struct {
	*Schedule
	index int
}

type indexedSchedules []indexedSchedule

// Ensure indexedSchedules implements sort.Interface.
var _ sort.Interface = make(indexedSchedules, 0)

// Len returns the length of this slice. Implements sort.Interface.
func (s indexedSchedules) Len() int {
	return len(s)
}

// Less returns whether the ith indexedSchedule sorts before the jth. Implements
// sort.Interface.
func (s indexedSchedules) Less(i, j int) bool {
	switch {
	case s[i].Schedule.Start.Day < s[j].Schedule.Start.Day:
		return true
	case s[i].Schedule.Start.Day > s[j].Schedule.Start.Day:
		return false
	default:
		ti, _ := s[i].Schedule.Start.ToTime()
		tj, _ := s[j].Schedule.Start.ToTime()
		return ti.Before(tj)
	}
}

// Swap swaps the ith indexedSchedule with the jth. Implements sort.Interface.
func (s indexedSchedules) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Validate validates this amount.
func (a *Amount) Validate(c *validation.Context) {
	if a.GetDefault() < 0 {
		c.Errorf("default amount must be non-negative")
	}
	schs := make(indexedSchedules, len(a.GetChange()))
	for i, ch := range a.GetChange() {
		c.Enter("change %d", i)
		ch.Validate(c)
		c.Exit()
		schs[i] = indexedSchedule{
			Schedule: ch,
			index:    i,
		}
	}
	sort.Sort(schs)
	// With schedules sorted by start time, checking for conflicts is only
	// necessary between adjacent schedules. Since schedules are relative to
	// the week, additionally check for a conflict between the last and first.
	if len(schs) > 0 {
		schs = append(schs, schs[0])
	}
	t := time.Time{}
	for i := 0; i < len(schs); i++ {
		s := schs[i]
		c.Enter("change %d", s.index)
		t1, err := s.Schedule.Start.ToTime()
		if i == len(schs)-1 {
			// The last schedule in schs is actually the first configured schedule again.
			// Treat this schedule as starting in a week. This checks for a conflict
			// between the last configured schedule and the first configured schedule.
			t1 = t1.Add(time.Hour * time.Duration(24*(s.Schedule.Start.GetDay()+7)))
		} else {
			t1 = t1.Add(time.Hour * time.Duration(24*s.Schedule.Start.GetDay()))
		}
		switch {
		case err != nil:
			c.Exit()
			return
		case t1.Before(t):
			// t1.Before(t) implies intervals are half-open: [start, end).
			// t is initialized to the zero time, whereas t1 is relative
			// to the current day. Therefore this can't succeed when i
			// is 0, meaning i-1 is never -1.
			c.Errorf("start time is before change %d", schs[i-1].index)
		}
		sec, err := s.Schedule.Length.ToSeconds()
		if err != nil {
			c.Exit()
			return
		}
		t = t1.Add(time.Second * time.Duration(sec))
		c.Exit()
	}
}
