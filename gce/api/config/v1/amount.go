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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
)

// getAmount returns the amount to use given the proposed amount and time.
// Assumes this amount has been validated.
func (a *Amount) getAmount(proposed int32, now time.Time) (int32, error) {
	for _, s := range a.GetChange() {
		start, err := s.mostRecentStart(now)
		if err != nil {
			return 0, errors.Fmt("invalid start time: %w", err)
		}
		if start.Before(now) || start.Equal(now) {
			sec, err := s.Length.ToSeconds()
			if err != nil {
				return 0, errors.Fmt("invalid length: %w", err)
			}
			end := start.Add(time.Second * time.Duration(sec))
			if now.Before(end) {
				if proposed < s.Min {
					return s.Min, nil
				}
				if proposed > s.Max {
					return s.Max, nil
				}
				return proposed, nil
			}
		}
	}
	if proposed < a.GetMin() {
		return a.GetMin(), nil
	}
	if proposed > a.GetMax() {
		return a.GetMax(), nil
	}
	return proposed, nil
}

// Validate validates this amount.
func (a *Amount) Validate(c *validation.Context) {
	if a.GetMin() < 0 {
		c.Errorf("minimum amount must be non-negative")
	}
	if a.GetMax() < 0 {
		c.Errorf("maximum amount must be non-negative")
	}
	if a.GetMin() > a.GetMax() {
		c.Errorf("minimum amount must not exceed maximum amount")
	}
	a.validateSchedules(c)
}

// indexedSchedule encapsulates *Schedules for sorting.
type indexedSchedule struct {
	*Schedule
	// index is the original index of the *Schedule before sorting.
	index int
	// sortKey is the time.Time by which []indexedSchedules should be sorted.
	sortKey time.Time
}

// validateSchedules validates the schedules in this amount.
func (a *Amount) validateSchedules(c *validation.Context) {
	// The algorithm works with any time 7 days greater than the zero time.
	now := time.Date(2018, 1, 1, 12, 0, 0, 0, time.UTC)
	schs := make([]indexedSchedule, len(a.GetChange()))
	for i, s := range a.GetChange() {
		c.Enter("change %d", i)
		s.Validate(c)
		c.Exit()
		t, err := s.mostRecentStart(now)
		if err != nil {
			// s.Validate(c) already emitted this error.
			return
		}
		schs[i] = indexedSchedule{
			Schedule: s,
			index:    i,
			sortKey:  t,
		}
	}
	sort.Slice(schs, func(i, j int) bool { return schs[i].sortKey.Before(schs[j].sortKey) })
	prevEnd := time.Time{}
	for i := range schs {
		c.Enter("change %d", schs[i].index)
		start := schs[i].sortKey
		if schs[i].sortKey.Before(prevEnd) {
			// Implies intervals are half-open: [start, end). prevEnd
			// is initialized to the zero time, therefore this can't
			// succeed when i is 0, meaning i-1 is never -1.
			c.Errorf("start time is before change %d", schs[i-1].index)
		}
		sec, err := schs[i].Schedule.Length.ToSeconds()
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
		c.Enter("change %d", schs[0].index)
		// Treat the first schedule as starting in a week. This checks for a conflict
		// between the last configured schedule and the first configured schedule.
		start := schs[0].sortKey.Add(time.Hour * time.Duration(24*7))
		if start.Before(prevEnd) {
			c.Errorf("start time is before change %d", schs[len(schs)-1].index)
		}
	}
}
