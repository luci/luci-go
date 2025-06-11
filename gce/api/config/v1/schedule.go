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
	"time"

	"google.golang.org/genproto/googleapis/type/dayofweek"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
)

// isSameDay returns whether the given time.Weekday and dayofweek.DayOfWeek
// represent the same day of the week.
func isSameDay(wkd time.Weekday, dow dayofweek.DayOfWeek) bool {
	switch dow {
	case dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED:
		return false
	case dayofweek.DayOfWeek_SUNDAY:
		// time.Weekday has Sunday == 0, dayofweek.DayOfWeek has Sunday == 7.
		return wkd == time.Sunday
	default:
		// For all other values, time.Weekday == dayofweek.DayOfWeek.
		return int(wkd) == int(dow)
	}
}

// mostRecentStart returns the most recent start time for this schedule relative
// to the given time. The date in the returned time.Time will be the most recent
// date falling on the day of the week specified in this schedule which results
// in the returned time being equal to or before the given time.
func (s *Schedule) mostRecentStart(now time.Time) (time.Time, error) {
	t, err := s.GetStart().toTime()
	if err != nil {
		return time.Time{}, err
	}
	now = now.In(t.Location())
	// toTime returns a time relative to the current date. Change it to be relative to the given date.
	t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	if s.GetStart().Day == dayofweek.DayOfWeek_DAY_OF_WEEK_UNSPECIFIED {
		return time.Time{}, errors.New("day must be specified")
	}
	for !isSameDay(t.Weekday(), s.GetStart().Day) {
		t = t.Add(time.Hour * -24)
	}
	if t.After(now) {
		t = t.Add(time.Hour * -24 * 7)
	}
	return t, nil
}

// Validate validates this schedule.
func (s *Schedule) Validate(c *validation.Context) {
	if s.GetMin() < 0 {
		c.Errorf("minimum amount must be non-negative")
	}
	if s.GetMax() < 0 {
		c.Errorf("maximum amount must be non-negative")
	}
	if s.GetMin() > s.GetMax() {
		c.Errorf("minimum amount must not exceed maximum amount")
	}
	c.Enter("length")
	s.GetLength().Validate(c)
	switch n, err := s.Length.ToSeconds(); {
	case err != nil:
		c.Errorf("%s", err)
	case n == 0:
		c.Errorf("duration or seconds is required")
	}
	c.Exit()
	c.Enter("start")
	s.GetStart().Validate(c)
	c.Exit()
}
