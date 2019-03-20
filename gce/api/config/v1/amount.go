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

// GetAmount returns the amount to use at the given time. Returns the first
// matching amount, which should be the only match if this *Amount has been
// validated.
func (a *Amount) GetAmount(now time.Time) (int32, error) {
	for _, s := range a.GetChange() {
		start, err := s.Start.toTime(now)
		if err != nil {
			return 0, errors.Annotate(err, "invalid start time").Err()
		}
		if start.Before(now) || start.Equal(now) {
			sec, err := s.Length.ToSeconds()
			if err != nil {
				return 0, errors.Annotate(err, "invalid length").Err()
			}
			end := start.Add(time.Second * time.Duration(sec))
			if now.Before(end) {
				return s.Amount, nil
			}
		}
	}
	return a.GetDefault(), nil
}

// Normalize ensures this amount's schedules are sorted.
func (a *Amount) Normalize() error {
	sort.Slice(a.GetChange(), func(i, j int) bool { return less(time.Time{}, a.Change[i], a.Change[j]) })
	return nil
}

// indexedSchedule encapsulates a *Schedule and its original index in the
// *Amount.
type indexedSchedule struct {
	*Schedule
	index int
}

// Validate validates this amount.
func (a *Amount) Validate(c *validation.Context) {
	if a.GetDefault() < 0 {
		c.Errorf("default amount must be non-negative")
	}
	schs := make([]indexedSchedule, len(a.GetChange()))
	for i, s := range a.GetChange() {
		c.Enter("change %d", i)
		s.Validate(c)
		c.Exit()
		schs[i] = indexedSchedule{
			Schedule: s,
			index:    i,
		}
	}
	// The algorithm below works as long as less and toTime use the same relative time, and that
	// relative time is more than 7 days past what prevEnd is initialized to. For determinism,
	// arbitrarily fix this relative time.
	now := time.Date(2018, 1, 1, 12, 0, 0, 0, time.UTC)
	sort.Slice(schs, func(i, j int) bool { return less(now, schs[i].Schedule, schs[j].Schedule) })
	prevEnd := time.Time{}
	for i := 0; i < len(schs); i++ {
		s := schs[i]
		c.Enter("change %d", s.index)
		start, err := s.Schedule.Start.toTime(now)
		if err != nil {
			// This error was already emitted by ch.Validate.
			c.Exit()
			return
		}
		if start.Before(prevEnd) {
			// Implies intervals are half-open: [start, end). prevEnd
			// is initialized to the zero time, therefore this can't
			// succeed when i is 0, meaning i-1 is never -1.
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
		start, err := s.Schedule.Start.toTime(now)
		if err != nil {
			c.Exit()
			return
		}
		// Treat the first schedule as starting in a week. This checks for a conflict
		// between the last configured schedule and the first configured schedule.
		start = start.Add(time.Hour * time.Duration(24*7))
		if start.Before(prevEnd) {
			c.Errorf("start time is before change %d", schs[len(schs)-1].index)
		}
	}
}
