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

package schedule

import (
	"context"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/type/dayofweek"
	"google.golang.org/genproto/googleapis/type/timeofday"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

func TestLoadTimezone(t *testing.T) {
	t.Parallel()

	ftt.Run("LoadTimezone", t, func(t *ftt.Test) {
		loc, err := LoadTimezone("America/Los_Angeles")
		assert.NoErr(t, err)
		assert.That(t, loc.String(), should.Equal("America/Los_Angeles"))
		// Cached.
		loc2, err := LoadTimezone("America/Los_Angeles")
		assert.NoErr(t, err)
		assert.That(t, loc2, should.Equal(loc))

		_, err = LoadTimezone("")
		assert.ErrIsLike(t, err, "missing")
		_, err = LoadTimezone("Local")
		assert.ErrIsLike(t, err, `"Local" is not allowed`)
		_, err = LoadTimezone("Mars/Olympus_Mons")
		assert.ErrIsLike(t, err, "unknown time zone")
	})
}

func TestValidateStartTime(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateStartTime", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			for _, tod := range []*timeofday.TimeOfDay{
				{},
				{Hours: 8, Minutes: 30},
				{Hours: 23, Minutes: 59},
			} {
				assert.NoErr(t, ValidateStartTime(tod))
			}
		})

		t.Run("invalid", func(t *ftt.Test) {
			fail := func(tod *timeofday.TimeOfDay, msg string) {
				assert.ErrIsLike(t, ValidateStartTime(tod), msg)
			}
			fail(nil, "missing")
			fail(&timeofday.TimeOfDay{Hours: 24}, "hours must be between 0 and 23; got 24")
			fail(&timeofday.TimeOfDay{Hours: -1}, "hours must be between 0 and 23; got -1")
			fail(&timeofday.TimeOfDay{Minutes: 60}, "minutes must be between 0 and 59; got 60")
			fail(&timeofday.TimeOfDay{Seconds: 1}, "seconds must not be set; got 1")
			fail(&timeofday.TimeOfDay{Nanos: 1}, "nanos must not be set; got 1")
		})
	})
}

func TestValidateDuration(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateDuration", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			for _, d := range []time.Duration{time.Second, 12 * time.Hour, MaxDuration} {
				assert.NoErr(t, ValidateDuration(durationpb.New(d)))
			}
		})

		t.Run("invalid", func(t *ftt.Test) {
			fail := func(d *durationpb.Duration, msg string) {
				assert.ErrIsLike(t, ValidateDuration(d), msg)
			}
			fail(nil, "missing")
			fail(&durationpb.Duration{Seconds: 1, Nanos: -1}, "invalid: ")
			fail(durationpb.New(0), "must be positive")
			fail(durationpb.New(-time.Hour), "must be positive")
			fail(durationpb.New(MaxDuration+time.Second), "must not exceed 24h0m0s")
		})
	})
}

func TestIsActive(t *testing.T) {
	t.Parallel()

	ftt.Run("IsActive", t, func(t *ftt.Test) {
		la, err := time.LoadLocation("America/Los_Angeles")
		assert.NoErr(t, err)
		weekdays := []dayofweek.DayOfWeek{
			dayofweek.DayOfWeek_MONDAY,
			dayofweek.DayOfWeek_TUESDAY,
			dayofweek.DayOfWeek_WEDNESDAY,
			dayofweek.DayOfWeek_THURSDAY,
			dayofweek.DayOfWeek_FRIDAY,
		}
		// Reference week: Mon Oct 19 .. Sun Oct 25, 2026.
		at := func(day, hour, minute int) time.Time {
			return time.Date(2026, time.October, day, hour, minute, 0, 0, la)
		}
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		check := func(s *cfgpb.UserLimit_Override_Schedule, now time.Time, want bool) {
			tc.Set(now)
			got, err := IsActive(ctx, s)
			assert.NoErr(t, err)
			assert.That(t, got, should.Equal(want), truth.Explain("at %s", now))
		}

		t.Run("same-day window is [start, start + duration)", func(t *ftt.Test) {
			s := &cfgpb.UserLimit_Override_Schedule{
				Timezone:  "America/Los_Angeles",
				Days:      weekdays,
				StartTime: &timeofday.TimeOfDay{Hours: 8},
				Duration:  durationpb.New(12 * time.Hour),
			}
			check(s, at(21, 7, 59), false)
			check(s, at(21, 8, 0), true)
			check(s, at(21, 19, 59), true)
			check(s, at(21, 20, 0), false)
			check(s, at(24, 12, 0), false) // Saturday.
		})

		t.Run("overnight window extends into the next day", func(t *ftt.Test) {
			s := &cfgpb.UserLimit_Override_Schedule{
				Timezone:  "America/Los_Angeles",
				Days:      weekdays,
				StartTime: &timeofday.TimeOfDay{Hours: 22},
				Duration:  durationpb.New(8 * time.Hour),
			}
			check(s, at(19, 3, 0), false)  // Mon early: window started Sunday.
			check(s, at(20, 3, 0), true)   // Tue early: window started Monday.
			check(s, at(23, 23, 0), true)  // Fri late.
			check(s, at(24, 3, 0), true)   // Sat early: window started Friday.
			check(s, at(24, 6, 0), false)  // Window end is exclusive.
			check(s, at(24, 23, 0), false) // Sat late: Saturday not in days.
		})

		t.Run("24h duration covers the whole day", func(t *ftt.Test) {
			s := &cfgpb.UserLimit_Override_Schedule{
				Timezone:  "America/Los_Angeles",
				Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_SATURDAY, dayofweek.DayOfWeek_SUNDAY},
				StartTime: &timeofday.TimeOfDay{},
				Duration:  durationpb.New(24 * time.Hour),
			}
			check(s, at(23, 23, 59), false) // Friday.
			check(s, at(24, 0, 0), true)
			check(s, at(25, 23, 59), true)
			check(s, at(26, 0, 0), false) // Monday.
		})

		t.Run("evaluated in the schedule timezone across DST", func(t *ftt.Test) {
			s := &cfgpb.UserLimit_Override_Schedule{
				Timezone:  "America/Los_Angeles",
				Days:      weekdays,
				StartTime: &timeofday.TimeOfDay{Hours: 9},
				Duration:  durationpb.New(8 * time.Hour),
			}
			// 16:00 UTC is 09:00 PDT in summer but 08:00 PST in winter.
			check(s, time.Date(2026, time.July, 15, 16, 0, 0, 0, time.UTC), true)
			check(s, time.Date(2026, time.November, 18, 16, 0, 0, 0, time.UTC), false)
			check(s, time.Date(2026, time.November, 18, 17, 0, 0, 0, time.UTC), true)
		})

		t.Run("window spanning a DST transition uses elapsed duration", func(t *ftt.Test) {
			// DST ends on Sunday Nov 1, 2026 at 02:00 PDT -> 01:00 PST, so a
			// 12h window starting Saturday 20:00 ends at 07:00 PST on Sunday.
			s := &cfgpb.UserLimit_Override_Schedule{
				Timezone:  "America/Los_Angeles",
				Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_SATURDAY},
				StartTime: &timeofday.TimeOfDay{Hours: 20},
				Duration:  durationpb.New(12 * time.Hour),
			}
			check(s, time.Date(2026, time.October, 31, 19, 59, 59, 0, la), false)
			check(s, time.Date(2026, time.October, 31, 20, 0, 0, 0, la), true)
			check(s, time.Date(2026, time.November, 1, 6, 59, 59, 0, la), true)
			check(s, time.Date(2026, time.November, 1, 7, 0, 0, 0, la), false)
		})

		t.Run("start_time in a DST gap is shifted forward by the gap", func(t *ftt.Test) {
			// Clocks spring forward on Sunday Mar 8, 2026 from 02:00 PST to
			// 03:00 PDT, so 02:30 doesn't exist and the window starts at 03:30 PDT.
			s := &cfgpb.UserLimit_Override_Schedule{
				Timezone:  "America/Los_Angeles",
				Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_SUNDAY},
				StartTime: &timeofday.TimeOfDay{Hours: 2, Minutes: 30},
				Duration:  durationpb.New(time.Hour),
			}
			check(s, time.Date(2026, time.March, 8, 10, 29, 0, 0, time.UTC), false) // 03:29 PDT.
			check(s, time.Date(2026, time.March, 8, 10, 30, 0, 0, time.UTC), true)  // 03:30 PDT.
			check(s, time.Date(2026, time.March, 8, 11, 29, 0, 0, time.UTC), true)  // 04:29 PDT.
			check(s, time.Date(2026, time.March, 8, 11, 30, 0, 0, time.UTC), false) // 04:30 PDT.
		})

		t.Run("ambiguous start_time uses the first occurrence", func(t *ftt.Test) {
			// Clocks fall back on Sunday Nov 1, 2026 from 02:00 PDT to 01:00 PST,
			// so 01:30 occurs twice: 08:30 UTC (PDT) and 09:30 UTC (PST).
			s := &cfgpb.UserLimit_Override_Schedule{
				Timezone:  "America/Los_Angeles",
				Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_SUNDAY},
				StartTime: &timeofday.TimeOfDay{Hours: 1, Minutes: 30},
				Duration:  durationpb.New(30 * time.Minute),
			}
			check(s, time.Date(2026, time.November, 1, 8, 29, 0, 0, time.UTC), false)
			check(s, time.Date(2026, time.November, 1, 8, 30, 0, 0, time.UTC), true)
			check(s, time.Date(2026, time.November, 1, 8, 59, 0, 0, time.UTC), true)
			check(s, time.Date(2026, time.November, 1, 9, 0, 0, 0, time.UTC), false)
			check(s, time.Date(2026, time.November, 1, 9, 45, 0, 0, time.UTC), false)
		})

		t.Run("window started two calendar days ago across a DST gap", func(t *ftt.Test) {
			// A 24h window starting Saturday Mar 7, 2026 at 23:30 PST spans the
			// spring-forward transition and ends at Monday Mar 9 00:30 PDT.
			s := &cfgpb.UserLimit_Override_Schedule{
				Timezone:  "America/Los_Angeles",
				Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_SATURDAY},
				StartTime: &timeofday.TimeOfDay{Hours: 23, Minutes: 30},
				Duration:  durationpb.New(24 * time.Hour),
			}
			check(s, time.Date(2026, time.March, 9, 0, 15, 0, 0, la), true)
			check(s, time.Date(2026, time.March, 9, 0, 30, 0, 0, la), false)
		})

		t.Run("malformed schedule", func(t *ftt.Test) {
			valid := func() *cfgpb.UserLimit_Override_Schedule {
				return &cfgpb.UserLimit_Override_Schedule{
					Timezone:  "UTC",
					Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_WEDNESDAY},
					StartTime: &timeofday.TimeOfDay{Hours: 8},
					Duration:  durationpb.New(12 * time.Hour),
				}
			}
			fail := func(s *cfgpb.UserLimit_Override_Schedule, msg string) {
				tc.Set(at(21, 12, 0))
				_, err := IsActive(ctx, s)
				assert.ErrIsLike(t, err, msg)
			}

			fail(nil, "schedule override missing")

			s := valid()
			s.Timezone = ""
			fail(s, `schedule override has invalid timezone "": missing`)

			s = valid()
			s.Timezone = "Invalid/Timezone"
			fail(s, `schedule override has invalid timezone "Invalid/Timezone": unknown time zone`)

			s = valid()
			s.Timezone = "Local"
			fail(s, `schedule override has invalid timezone "Local": "Local" is not allowed`)

			s = valid()
			s.StartTime = &timeofday.TimeOfDay{Hours: 24}
			fail(s, "schedule override has invalid start_time: hours must be between 0 and 23")

			s = valid()
			s.Duration = nil
			fail(s, "schedule override has invalid duration: missing")

			s = valid()
			s.Duration = durationpb.New(25 * time.Hour)
			fail(s, "schedule override has invalid duration: must not exceed")
		})
	})
}
