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

// less returns whether the first *Schedule sorts before the second.
// Returns false if either *Schedule's time can't be parsed.
func less(si, sj *Schedule) bool {
	switch {
	case si.GetStart().GetDay() < sj.GetStart().GetDay():
		return true
	case si.GetStart().GetDay() > sj.GetStart().GetDay():
		return false
	default:
		ti, err := si.GetStart().ToTime()
		if err != nil {
			return false
		}
		tj, err := sj.GetStart().ToTime()
		if err != nil {
			return false
		}
		return ti.Before(tj)
	}
}

// Validate validates this amount.
func (a *Amount) Validate(c *validation.Context) {
	if a.GetDefault() < 0 {
		c.Errorf("default amount must be non-negative")
	}
	schs := make([]indexedSchedule, len(a.GetChange()))
	for i, ch := range a.GetChange() {
		c.Enter("change %d", i)
		ch.Validate(c)
		c.Exit()
		schs[i] = indexedSchedule{
			Schedule: ch,
			index:    i,
		}
	}
	sort.Slice(schs, func(i, j int) bool { return less(schs[i].Schedule, schs[j].Schedule) })
	prevEnd := time.Time{}
	for i := 0; i < len(schs); i++ {
		s := schs[i]
		c.Enter("change %d", s.index)
		start, err := s.Schedule.Start.ToTime()
		if err != nil {
			// This error was already emitted by ch.Validate.
			c.Exit()
			return
		}
		start = start.Add(time.Hour * time.Duration(24*s.Schedule.Start.GetDay()))
		if start.Before(prevEnd) {
			// Implies intervals are half-open: [start, end).
			// prevEnd is initialized to the zero time, whereas start is
			// relative to the current day. Therefore this can't succeed
			// when i is 0, meaning i-1 is never -1.
			c.Errorf("start time is before change %d", schs[i-1].index)
		}
		sec, err := s.Schedule.Length.ToSeconds()
		if err != nil {
			c.Exit()
			return
		}
		prevEnd = start.Add(time.Second * time.Duration(sec))
		c.Exit()
	}
	if len(schs) > 0 {
		// Schedules are relative to the week.
		// Check for a conflict between the last and first.
		s := schs[0]
		c.Enter("change %d", s.index)
		start, err := s.Schedule.Start.ToTime()
		if err != nil {
			c.Exit()
			return
		}
		// Treat the first schedule as starting in a week. This checks for a conflict
		// between the last configured schedule and the first configured schedule.
		start = start.Add(time.Hour * time.Duration(24*(s.Schedule.Start.GetDay()+7)))
		if start.Before(prevEnd) {
			c.Errorf("start time is before change %d", schs[len(schs)-1].index)
		}
	}
}
