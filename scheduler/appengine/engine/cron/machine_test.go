// Copyright 2017 The LUCI Authors.
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

package cron

import (
	"testing"
	"time"

	"go.chromium.org/luci/scheduler/appengine/schedule"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMachine(t *testing.T) {
	t.Parallel()

	at0min, _ := schedule.Parse("0 * * * *", 0)
	at45min, _ := schedule.Parse("45 * * * *", 0)
	each30min, _ := schedule.Parse("with 30m interval", 0)
	each10min, _ := schedule.Parse("with 10m interval", 0)
	never, _ := schedule.Parse("triggered", 0)

	Convey("Absolute schedule", t, func() {
		tm := testMachine{
			Now:      parseTime("00:15"),
			Schedule: at0min,
		}

		// Enabling the job schedules the first tick based on the schedule.
		err := tm.roll(func(m *Machine) error {
			m.Enable()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			TickLaterAction{
				When:      parseTime("01:00"),
				TickNonce: 1,
			},
		})

		// RewindIfNecessary does nothing, the tick is already set.
		err = tm.roll(func(m *Machine) error {
			m.RewindIfNecessary()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)

		// Very early tick is ignored with an error.
		tm.Now = parseTime("01:00").Add(-200 * time.Millisecond)
		err = tm.roll(func(m *Machine) error { return m.OnTimerTick(1) })
		So(err.Error(), ShouldEqual, "tick happened 200ms before it was expected")
		So(tm.Actions, ShouldEqual, nil)

		// Slightly earlier tick (i.e. due to clock desync) is accepted.
		tm.Now = parseTime("01:00").Add(-20 * time.Millisecond)

		// A tick with wrong nonce is silently skipped.
		err = tm.roll(func(m *Machine) error { return m.OnTimerTick(123) })
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldEqual, nil)

		// The correct tick comes. Invocation is started and new tick is scheduled.
		err = tm.roll(func(m *Machine) error { return m.OnTimerTick(1) })
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			StartInvocationAction{Generation: 3},
			TickLaterAction{
				When:      parseTime("02:00"),
				TickNonce: 2,
			},
		})

		// Disabling the job.
		err = tm.roll(func(m *Machine) error {
			m.Disable()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)

		// It silently skips the tick now.
		tm.Now = parseTime("02:00")
		err = tm.roll(func(m *Machine) error { return m.OnTimerTick(2) })
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldEqual, nil)
	})

	Convey("Relative schedule", t, func() {
		tm := testMachine{
			Now:      parseTime("00:00"),
			Schedule: each30min,
		}

		// Enabling the job schedules the first tick based on the schedule.
		err := tm.roll(func(m *Machine) error {
			m.Enable()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			TickLaterAction{
				When:      parseTime("00:30"),
				TickNonce: 1,
			},
		})

		// RewindIfNecessary does nothing, the tick is already set.
		err = tm.roll(func(m *Machine) error {
			m.RewindIfNecessary()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)

		// Tick arrives (slightly late). The invocation is started, but the next
		// tick is _not_ set.
		tm.Now = parseTime("00:31")
		err = tm.roll(func(m *Machine) error { return m.OnTimerTick(1) })
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			StartInvocationAction{Generation: 3},
		})

		// Some time later (when invocation has presumably finished), rewind the
		// clock. It sets a new tick 30min from now.
		tm.Now = parseTime("00:40")
		err = tm.roll(func(m *Machine) error {
			m.RewindIfNecessary()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			TickLaterAction{
				When:      parseTime("01:10"), // 40min + 30min
				TickNonce: 2,
			},
		})
	})

	Convey("Relative schedule, distant future", t, func() {
		tm := testMachine{
			Now:      parseTime("00:00"),
			Schedule: never,
		}

		// Enabling the job does nothing.
		err := tm.roll(func(m *Machine) error {
			m.Enable()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)

		// Rewinding does nothing.
		err = tm.roll(func(m *Machine) error {
			m.RewindIfNecessary()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)

		// Ticking does nothing.
		err = tm.roll(func(m *Machine) error { return m.OnTimerTick(1) })
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)
	})

	Convey("Schedule changes", t, func() {
		// Start with absolute.
		tm := testMachine{
			Now:      parseTime("00:00"),
			Schedule: at0min,
		}

		// The first tick is scheduled to 1h from now.
		err := tm.roll(func(m *Machine) error {
			m.Enable()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			TickLaterAction{
				When:      parseTime("01:00"),
				TickNonce: 1,
			},
		})

		// 10 min later switch to the relative schedule. It reschedules the tick
		// to 30 min since the _previous action_ (which was 'Enable').
		tm.Now = parseTime("00:10")
		tm.Schedule = each30min
		err = tm.roll(func(m *Machine) error {
			m.OnScheduleChange()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			TickLaterAction{
				When:      parseTime("00:30"),
				TickNonce: 2,
			},
		})

		// The operation is idempotent. No new tick is scheduled when we try again.
		tm.Now = parseTime("00:15")
		err = tm.roll(func(m *Machine) error {
			m.OnScheduleChange()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)

		// The scheduled tick comes. Since it is a relative schedule, no new tick
		// is scheduled.
		tm.Now = parseTime("00:30")
		err = tm.roll(func(m *Machine) error { return m.OnTimerTick(2) })
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			StartInvocationAction{Generation: 4},
		})

		// Some time later we switch it to another relative schedule. Nothing
		// happens, since we are waiting for a rewind now anyway.
		tm.Now = parseTime("00:40")
		tm.Schedule = each10min
		err = tm.roll(func(m *Machine) error {
			m.OnScheduleChange()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)

		// Now we switch back to the absolute schedule. It schedules a new tick.
		tm.Now = parseTime("01:30")
		tm.Schedule = at0min
		err = tm.roll(func(m *Machine) error {
			m.OnScheduleChange()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			TickLaterAction{
				When:      parseTime("02:00"),
				TickNonce: 3,
			},
		})

		// Changing the absolute schedule moves the tick accordingly.
		tm.Schedule = at45min
		err = tm.roll(func(m *Machine) error {
			m.OnScheduleChange()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			TickLaterAction{
				When:      parseTime("01:45"),
				TickNonce: 4,
			},
		})

		// Switching to 'triggered' schedule "disarms" the current tick by replacing
		// it with "tick in the distant future". This doesn't emit any actions,
		// since we can't actually schedule tick in the distant future.
		tm.Schedule = never
		err = tm.roll(func(m *Machine) error {
			m.OnScheduleChange()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)
		So(tm.State.LastTick, ShouldResemble, TickLaterAction{
			When:      schedule.DistantFuture,
			TickNonce: 5,
		})

		// Enabling back absolute schedule places a new tick.
		tm.Now = parseTime("01:30")
		tm.Schedule = at0min
		err = tm.roll(func(m *Machine) error {
			m.OnScheduleChange()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldResemble, []Action{
			TickLaterAction{
				When:      parseTime("02:00"),
				TickNonce: 6,
			},
		})

		// Schedule changes do nothing to disabled jobs.
		tm.Schedule = at45min
		err = tm.roll(func(m *Machine) error {
			m.Disable()
			m.OnScheduleChange()
			return nil
		})
		So(err, ShouldBeNil)
		So(tm.Actions, ShouldBeNil)
	})

	Convey("Equals uses time.Equal", t, func() {
		s1 := State{
			Enabled:    true,
			Generation: 123,
			LastRewind: parseTime("00:15"),
		}
		s2 := State{
			Enabled:    true,
			Generation: 123,
			LastRewind: parseTime("00:15").Local(), // same time, different TZ
		}
		So(s1.Equal(&s2), ShouldBeTrue)
	})

	Convey("Petty code coverage", t, func() {
		// Just to get 100% code coverage...
		So((StartInvocationAction{}).IsAction(), ShouldBeTrue)
		So((TickLaterAction{}).IsAction(), ShouldBeTrue)
	})
}

func parseTime(str string) time.Time {
	t, err := time.Parse(time.RFC822, "01 Jan 17 "+str+" UTC")
	if err != nil {
		panic(err)
	}
	return t
}

type testMachine struct {
	State    State
	Schedule *schedule.Schedule
	Now      time.Time
	Nonces   int64
	Actions  []Action
}

func (t *testMachine) roll(cb func(*Machine) error) error {
	m := Machine{
		Now:      t.Now,
		Schedule: t.Schedule,
		Nonce: func() int64 {
			t.Nonces++
			return t.Nonces
		},
		State: t.State,
	}

	if err := cb(&m); err != nil {
		return err
	}

	t.State = m.State
	t.Actions = m.Actions
	return nil
}
