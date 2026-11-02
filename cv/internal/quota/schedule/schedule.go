// Copyright 2026 The LUCI Authors.
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

// Package schedule evaluates and validates UserLimit override schedules.
package schedule

import (
	"context"
	"slices"
	"sync"
	"time"

	"google.golang.org/genproto/googleapis/type/dayofweek"
	"google.golang.org/genproto/googleapis/type/timeofday"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

// MaxDuration is the maximum allowed duration of a schedule's time window.
//
// IsActive relies on this bound: it only considers windows that started on
// the current or one of the two preceding calendar days. Raising this limit
// requires updating IsActive accordingly.
const MaxDuration = 24 * time.Hour

// locationCache caches *time.Location by IANA timezone name, since
// time.LoadLocation reads and parses tzdata on every call.
var locationCache sync.Map // string -> *time.Location

// LoadTimezone loads the location for an IANA timezone name of a schedule.
//
// Unlike time.LoadLocation, it rejects "" and "Local", which would make the
// schedule depend on the environment CV runs in.
func LoadTimezone(tz string) (*time.Location, error) {
	switch tz {
	case "":
		return nil, errors.New("missing")
	case "Local":
		return nil, errors.New(`"Local" is not allowed; must be an IANA timezone (e.g. "America/Los_Angeles" or "UTC")`)
	}
	if loc, ok := locationCache.Load(tz); ok {
		return loc.(*time.Location), nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, err
	}
	locationCache.Store(tz, loc)
	return loc, nil
}

// ValidateStartTime validates the start_time of a schedule.
//
// Only hours and minutes may be set.
func ValidateStartTime(t *timeofday.TimeOfDay) error {
	switch {
	case t == nil:
		return errors.New("missing")
	case t.GetHours() < 0 || t.GetHours() > 23:
		return errors.Fmt("hours must be between 0 and 23; got %d", t.GetHours())
	case t.GetMinutes() < 0 || t.GetMinutes() > 59:
		return errors.Fmt("minutes must be between 0 and 59; got %d", t.GetMinutes())
	case t.GetSeconds() != 0:
		return errors.Fmt("seconds must not be set; got %d", t.GetSeconds())
	case t.GetNanos() != 0:
		return errors.Fmt("nanos must not be set; got %d", t.GetNanos())
	}
	return nil
}

// ValidateDuration validates the duration of a schedule.
func ValidateDuration(d *durationpb.Duration) error {
	switch {
	case d == nil:
		return errors.New("missing")
	case d.CheckValid() != nil:
		return errors.Fmt("invalid: %w", d.CheckValid())
	case d.AsDuration() <= 0:
		return errors.New("must be positive")
	case d.AsDuration() > MaxDuration:
		return errors.Fmt("must not exceed %s", MaxDuration)
	}
	return nil
}

// IsActive reports whether the schedule's time window contains the current
// time, as reported by clock.Now(ctx).
//
// The schedule is active if the current time is within
// [start, start + duration) for any window that starts at `start_time` on one
// of the schedule's `days`, in the schedule's timezone. See windowStart for
// how start times affected by DST transitions are resolved.
//
// Returns an error if the schedule is malformed. Schedules in a validated
// config are never malformed.
func IsActive(ctx context.Context, s *cfgpb.UserLimit_Override_Schedule) (bool, error) {
	if s == nil {
		return false, errors.New("schedule override missing")
	}
	loc, err := LoadTimezone(s.GetTimezone())
	if err != nil {
		return false, errors.Fmt("schedule override has invalid timezone %q: %w", s.GetTimezone(), err)
	}
	st := s.GetStartTime()
	if err := ValidateStartTime(st); err != nil {
		return false, errors.Fmt("schedule override has invalid start_time: %w", err)
	}
	if err := ValidateDuration(s.GetDuration()); err != nil {
		return false, errors.Fmt("schedule override has invalid duration: %w", err)
	}
	dur := s.GetDuration().AsDuration()

	now := clock.Now(ctx)
	localNow := now.In(loc)
	// Since duration is at most MaxDuration (24h), only windows starting today,
	// yesterday or, when a DST transition shortens the elapsed day, the day
	// before yesterday can contain `now`.
	for daysAgo := range 3 {
		// Normalize the calendar date (e.g. March 0 -> February 28).
		date := time.Date(localNow.Year(), localNow.Month(), localNow.Day()-daysAgo, 0, 0, 0, 0, time.UTC)
		if !slices.Contains(s.GetDays(), toDayOfWeek(date.Weekday())) {
			continue
		}
		start := windowStart(date, int(st.GetHours()), int(st.GetMinutes()), loc)
		if !now.Before(start) && now.Before(start.Add(dur)) {
			return true, nil
		}
	}
	return false, nil
}

// windowStart returns the instant at which a window starting at hour:minute
// local time on the given calendar date (year, month and day of `date`) starts
// in loc.
//
// Unlike time.Date, which doesn't guarantee which instant is returned for
// local times affected by DST transitions, it resolves them deterministically:
//   - If the local time occurs twice (e.g. 01:30 when clocks fall back from
//     02:00 to 01:00), the first occurrence is used.
//   - If the local time doesn't exist (e.g. 02:30 when clocks spring forward
//     from 02:00 to 03:00), it is interpreted using the UTC offset in effect
//     before the transition, which shifts it forward by the DST gap
//     (e.g. 02:30 -> 03:30).
func windowStart(date time.Time, hour, minute int, loc *time.Location) time.Time {
	// The local wall-clock time, expressed as if it were UTC.
	wall := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, time.UTC)
	// UTC offsets in effect well before and well after the wall-clock time.
	_, offBefore := wall.Add(-24 * time.Hour).In(loc).Zone()
	_, offAfter := wall.Add(24 * time.Hour).In(loc).Zone()
	before := wall.Add(-time.Duration(offBefore) * time.Second)
	after := wall.Add(-time.Duration(offAfter) * time.Second)
	isWall := func(t time.Time) bool {
		l := t.In(loc)
		return l.Hour() == hour && l.Minute() == minute
	}
	switch beforeOK, afterOK := isWall(before), isWall(after); {
	case beforeOK && afterOK:
		// Ambiguous (or no transition): use the first occurrence.
		if after.Before(before) {
			return after
		}
		return before
	case afterOK:
		return after
	default:
		// Either only `before` is valid, or the local time doesn't exist.
		return before
	}
}

// toDayOfWeek converts a time.Weekday (Sunday=0) to a google.type.DayOfWeek
// (Monday=1, ..., Sunday=7).
func toDayOfWeek(wd time.Weekday) dayofweek.DayOfWeek {
	if wd == time.Sunday {
		return dayofweek.DayOfWeek_SUNDAY
	}
	return dayofweek.DayOfWeek(wd)
}
