// Copyright 2015 The LUCI Authors.
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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
	"time"
)

var (
	epoch           = parseTime("2015-09-14 22:42:00 +0000 UTC")
	closestMidnight = parseTime("2015-09-15 00:00:00 +0000 UTC")
)

func parseTime(t string) time.Time {
	tm, err := time.Parse("2006-01-02 15:04:05.999 -0700 MST", t)
	if err != nil {
		panic(err)
	}
	return tm
}

func timeTable(cron string, now time.Time, count int) (out []time.Time) {
	s, err := Parse(cron, 0)
	if err != nil {
		panic(err)
	}
	prev := time.Time{}
	for ; count > 0; count-- {
		next := s.Next(now, prev)
		out = append(out, next)
		prev = next
		now = next.Add(time.Millisecond)
	}
	return
}

func TestAbsoluteSchedule(t *testing.T) {
	t.Parallel()

	ftt.Run("Parsing success", t, func(t *ftt.Test) {
		sched, err := Parse("* * * * * *", 0)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, sched.String(), should.Equal("* * * * * *"))
	})

	ftt.Run("Parsing error", t, func(t *ftt.Test) {
		sched, err := Parse("not a schedule", 0)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, sched, should.BeNil)
	})

	ftt.Run("Next works", t, func(t *ftt.Test) {
		sched, _ := Parse("*/15 * * * * * *", 0)
		assert.Loosely(t, sched.IsAbsolute(), should.BeTrue)
		assert.Loosely(t, sched.Next(epoch, time.Time{}), should.Resemble(epoch.Add(15*time.Second)))
		assert.Loosely(t, sched.Next(epoch.Add(15*time.Second), epoch), should.Resemble(epoch.Add(30*time.Second)))
	})

	ftt.Run("Each 3 hours time table", t, func(t *ftt.Test) {
		assert.Loosely(t, timeTable("0 */3 * * * *", epoch, 4), should.Resemble([]time.Time{
			closestMidnight,
			closestMidnight.Add(3 * time.Hour),
			closestMidnight.Add(6 * time.Hour),
			closestMidnight.Add(9 * time.Hour),
		}))
		assert.Loosely(t, timeTable("0 1/3 * * * *", epoch, 4), should.Resemble([]time.Time{
			closestMidnight.Add(1 * time.Hour),
			closestMidnight.Add(4 * time.Hour),
			closestMidnight.Add(7 * time.Hour),
			closestMidnight.Add(10 * time.Hour),
		}))
	})

	ftt.Run("Trailing stars are optional", t, func(t *ftt.Test) {
		// Exact same time tables.
		assert.Loosely(t, timeTable("0 */3 * * *", epoch, 4), should.Resemble(timeTable("0 */3 * * * *", epoch, 4)))
	})

	ftt.Run("List of hours", t, func(t *ftt.Test) {
		assert.Loosely(t, timeTable("0 2,10,18 * * *", epoch, 4), should.Resemble([]time.Time{
			closestMidnight.Add(2 * time.Hour),
			closestMidnight.Add(10 * time.Hour),
			closestMidnight.Add(18 * time.Hour),
			closestMidnight.Add(26 * time.Hour),
		}))
	})

	ftt.Run("Once a day", t, func(t *ftt.Test) {
		assert.Loosely(t, timeTable("0 7 * * *", epoch, 4), should.Resemble([]time.Time{
			closestMidnight.Add(7 * time.Hour),
			closestMidnight.Add(31 * time.Hour),
			closestMidnight.Add(55 * time.Hour),
			closestMidnight.Add(79 * time.Hour),
		}))
	})
}

func TestRelativeSchedule(t *testing.T) {
	t.Parallel()

	ftt.Run("Parsing success", t, func(t *ftt.Test) {
		sched, err := Parse("with 15s interval", 0)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, sched.String(), should.Equal("with 15s interval"))
	})

	ftt.Run("Parsing error", t, func(t *ftt.Test) {
		sched, err := Parse("with bladasdafier", 0)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, sched, should.BeNil)

		sched, err = Parse("with -1s interval", 0)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, sched, should.BeNil)

		sched, err = Parse("with NaNs interval", 0)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, sched, should.BeNil)
	})

	ftt.Run("Next works", t, func(t *ftt.Test) {
		sched, _ := Parse("with 15s interval", 0)
		assert.Loosely(t, sched.IsAbsolute(), should.BeFalse)

		// First tick is pseudorandom.
		assert.Loosely(t, sched.Next(epoch, time.Time{}), should.Resemble(epoch.Add(14*time.Second+177942239*time.Nanosecond)))

		// Next tick is 15s from prev one, or now if it's too late.
		assert.Loosely(t, sched.Next(epoch.Add(16*time.Second), epoch.Add(15*time.Second)), should.Resemble(epoch.Add(30*time.Second)))
		assert.Loosely(t, sched.Next(epoch.Add(31*time.Second), epoch.Add(15*time.Second)), should.Resemble(epoch.Add(31*time.Second)))
	})
}
