// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package engine

import (
	"testing"
	"time"

	"github.com/luci/luci-go/scheduler/appengine/schedule"
	. "github.com/smartystreets/goconvey/convey"
)

var epoch = time.Unix(1442270520, 0).UTC()

func TestStateMachine(t *testing.T) {
	Convey("Normal flow on abs schedule", t, func() {
		m := newTestStateMachine("*/5 * * * * * *")

		// Noop when in disabled state.
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(1) })
		So(m.state.State, ShouldEqual, JobStateDisabled)
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarting(1, 1, 0) })
		So(m.state.State, ShouldEqual, JobStateDisabled)
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarted(1) })
		So(m.state.State, ShouldEqual, JobStateDisabled)
		m.roll(func(sm *StateMachine) { sm.OnInvocationDone(1) })
		So(m.state.State, ShouldEqual, JobStateDisabled)
		m.roll(func(sm *StateMachine) { sm.OnScheduleChange() })
		So(m.state.State, ShouldEqual, JobStateDisabled)

		// Enabling schedules a tick.
		m.roll(func(sm *StateMachine) { sm.OnJobEnabled() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(5 * time.Second), 1},
		})
		m.actions = nil

		// Re enabling does nothing.
		m.roll(func(sm *StateMachine) { sm.OnJobEnabled() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldBeNil)

		// Wrong tick ID is skipped.
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(2) })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldBeNil)

		// Early tick causes retry.
		So(m.rollWithErr(func(sm *StateMachine) error { return sm.OnTimerTick(1) }), ShouldNotBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldBeNil)

		// Tick on time moves to JobStateQueued, schedules next tick.
		m.now = m.now.Add(5 * time.Second)
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(1) })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(10 * time.Second), 2},
			StartInvocationAction{InvocationNonce: 3},
		})
		m.actions = nil

		// Wrong invocation nonce is skipped.
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarting(333, 1000, 0) })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.actions, ShouldBeNil)

		// Time to run.
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarting(3, 1000, 0) })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.state.InvocationID, ShouldEqual, 1000)

		// Skip wrong invocation ID.
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarted(1001) })
		So(m.state.State, ShouldEqual, JobStateQueued)

		// Started.
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarted(1000) })
		So(m.state.State, ShouldEqual, JobStateRunning)

		// Skip wrong invocation ID.
		m.roll(func(sm *StateMachine) { sm.OnInvocationDone(1001) })
		So(m.state.State, ShouldEqual, JobStateRunning)

		// End of the cycle. Ends up in scheduled state, waiting for the tick added
		// when StartInvocationAction was issued.
		m.roll(func(sm *StateMachine) { sm.OnInvocationDone(1000) })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 2)

		// Disable cancels the timer.
		m.roll(func(sm *StateMachine) { sm.OnJobDisabled() })
		So(m.state.State, ShouldEqual, JobStateDisabled)
		So(m.state.InvocationNonce, ShouldEqual, 0)
	})

	Convey("Overrun when queued", t, func() {
		m := newTestStateMachine("*/5 * * * * * *")

		// Enabling schedules a tick.
		m.roll(func(sm *StateMachine) { sm.OnJobEnabled() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(5 * time.Second), 1},
		})
		m.actions = nil

		// Tick on time moves to JobStateQueued
		m.now = m.now.Add(5 * time.Second)
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(1) })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(10 * time.Second), 2},
			StartInvocationAction{InvocationNonce: 3},
		})
		m.actions = nil

		// Overrun when queued.
		m.now = m.now.Add(5 * time.Second)
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(2) })
		So(m.state.State, ShouldEqual, JobStateSlowQueue)
		So(m.state.Overruns, ShouldEqual, 1)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(15 * time.Second), 4},
			RecordOverrunAction{Overruns: 1},
		})
		m.actions = nil

		// Time to run. Moves to JobStateOverrun because was stuck in queue.
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarting(3, 100, 0) })
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarted(100) })
		So(m.state.State, ShouldEqual, JobStateOverrun)
		So(m.state.Overruns, ShouldEqual, 1)

		// End of the cycle.
		m.roll(func(sm *StateMachine) { sm.OnInvocationDone(100) })
		So(m.state.State, ShouldEqual, JobStateScheduled)
	})

	Convey("Overrun when running", t, func() {
		m := newTestStateMachine("*/5 * * * * * *")

		// Enabling schedules a tick.
		m.roll(func(sm *StateMachine) { sm.OnJobEnabled() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(5 * time.Second), 1},
		})
		m.actions = nil

		// Tick on time moves to JobStateQueued
		m.now = m.now.Add(5 * time.Second)
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(1) })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(10 * time.Second), 2},
			StartInvocationAction{InvocationNonce: 3},
		})
		m.actions = nil

		// Time to run.
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarting(3, 100, 0) })
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarted(100) })
		So(m.state.State, ShouldEqual, JobStateRunning)

		// Next tick comes while job is running.
		m.now = m.now.Add(5 * time.Second)
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(2) })
		So(m.state.State, ShouldEqual, JobStateOverrun)
		So(m.state.Overruns, ShouldEqual, 1)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(15 * time.Second), 4},
			RecordOverrunAction{Overruns: 1, RunningInvocationID: 100},
		})
		m.actions = nil

		// End of the cycle.
		m.roll(func(sm *StateMachine) { sm.OnInvocationDone(100) })
		So(m.state.State, ShouldEqual, JobStateScheduled)
	})

	Convey("Normal flow on rel schedule", t, func() {
		m := newTestStateMachine("with 10s interval")

		// Enabling schedules the first tick at some random moment in time.
		m.roll(func(sm *StateMachine) { sm.OnJobEnabled() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(9*time.Second + 451961492*time.Nanosecond), 1},
		})
		m.actions = nil

		// Tick on time moves to JobStateQueued and does NOT schedule a tick.
		m.now = m.now.Add(9*time.Second + 451961492*time.Nanosecond)
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(1) })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.state.TickNonce, ShouldEqual, 0)
		So(m.actions, ShouldResemble, []Action{
			StartInvocationAction{InvocationNonce: 2},
		})
		m.actions = nil

		// Time to run.
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarting(2, 1000, 0) })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.state.InvocationID, ShouldEqual, 1000)

		// Started.
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarted(1000) })
		So(m.state.State, ShouldEqual, JobStateRunning)

		// Let it run for some time.
		m.now = epoch.Add(20 * time.Second)

		// End of the cycle. New tick is scheduled, 10s from current time.
		m.roll(func(sm *StateMachine) { sm.OnInvocationDone(1000) })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 3)
		So(m.state.TickTime, ShouldResemble, m.now.Add(10*time.Second))
	})

	Convey("Schedule changes", t, func() {
		m := newTestStateMachine("*/5 * * * * * *")

		// Enabling schedules a tick after 5 sec.
		m.roll(func(sm *StateMachine) { sm.OnJobEnabled() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 1)
		So(m.state.TickTime, ShouldResemble, epoch.Add(5*time.Second))
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(5 * time.Second), 1},
		})
		m.actions = nil

		// Rescheduling event with same next tick time is noop.
		m.roll(func(sm *StateMachine) { sm.OnScheduleChange() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 1)
		So(m.state.TickTime, ShouldResemble, epoch.Add(5*time.Second))
		So(m.actions, ShouldBeNil)

		// Suddenly ticking each second.
		sched, _ := schedule.Parse("*/1 * * * * * *", 0)
		m.schedule = sched

		// Should be rescheduled to run earlier
		m.roll(func(sm *StateMachine) { sm.OnScheduleChange() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 2)
		So(m.state.TickTime, ShouldResemble, epoch.Add(1*time.Second))
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(1 * time.Second), 2},
		})
		m.actions = nil

		// Starts running.
		m.now = m.now.Add(time.Second)
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(2) })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.state.InvocationNonce, ShouldEqual, 4)
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarting(4, 123, 0) })
		m.roll(func(sm *StateMachine) { sm.OnInvocationStarted(123) })
		So(m.state.State, ShouldEqual, JobStateRunning)

		// Next tick is scheduled based on absolute schedule.
		So(m.state.TickNonce, ShouldEqual, 3)
		So(m.state.TickTime, ShouldResemble, epoch.Add(2*time.Second))
		m.actions = nil

		// Switching to use relative schedule (while job is still running).
		sched, _ = schedule.Parse("with 20s interval", 0)
		m.schedule = sched

		// Cancels pending tick.
		m.roll(func(sm *StateMachine) { sm.OnScheduleChange() })
		So(m.state.State, ShouldEqual, JobStateRunning)
		So(m.state.TickNonce, ShouldEqual, 0)
		So(m.actions, ShouldBeNil)

		// Switching back to absolute schedule.
		sched, _ = schedule.Parse("*/1 * * * * * *", 0)
		m.schedule = sched

		// Reschedules the tick again.
		m.roll(func(sm *StateMachine) { sm.OnScheduleChange() })
		So(m.state.State, ShouldEqual, JobStateRunning)
		So(m.state.TickNonce, ShouldEqual, 5)
		So(m.state.TickTime, ShouldResemble, epoch.Add(2*time.Second))
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(2 * time.Second), 5},
		})
		m.actions = nil
	})

	Convey("OnManualInvocation works with abs schedule", t, func() {
		m := newTestStateMachine("*/5 * * * * * *")

		// Enabling schedules a tick after 5 sec.
		m.roll(func(sm *StateMachine) { sm.OnJobEnabled() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(5 * time.Second), 1},
		})
		m.actions = nil

		// Asking to run the job works. It doesn't touch job's schedule.
		m.roll(func(sm *StateMachine) { sm.OnManualInvocation("user:abc") })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.actions, ShouldResemble, []Action{
			StartInvocationAction{
				InvocationNonce: 2,
				TriggeredBy:     "user:abc",
			},
		})
		m.actions = nil

		// Second call doesn't work. The job is queued already.
		err := m.rollWithErr(func(sm *StateMachine) error { return sm.OnManualInvocation("user:abc") })
		So(err, ShouldNotBeNil)
	})

	Convey("OnManualInvocation works with rel schedule", t, func() {
		m := newTestStateMachine("with 5s interval")

		// Enabling schedules a tick after random amount of seconds.
		m.roll(func(sm *StateMachine) { sm.OnJobEnabled() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 1)
		m.actions = nil

		// Asking to run the job works. It resets the tick (to be enabled again when
		// job finishes).
		m.roll(func(sm *StateMachine) { sm.OnManualInvocation("user:abc") })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.state.TickNonce, ShouldEqual, 0) // reset
		So(m.actions, ShouldResemble, []Action{
			StartInvocationAction{
				InvocationNonce: 2,
				TriggeredBy:     "user:abc",
			},
		})

		m.actions = nil

		// Second call doesn't work. The job is queued already.
		err := m.rollWithErr(func(sm *StateMachine) error { return sm.OnManualInvocation("user:abc") })
		So(err, ShouldNotBeNil)
	})

	Convey("OnManualAbort works with rel schedule", t, func() {
		m := newTestStateMachine("with 5s interval")

		// Enabling schedules a tick after random amount of seconds.
		m.roll(func(sm *StateMachine) { sm.OnJobEnabled() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 1)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(4*time.Second + 725980746*time.Nanosecond), 1},
		})
		m.actions = nil

		// Nothing to abort yet. Doesn't change the state.
		m.roll(func(sm *StateMachine) { sm.OnManualAbort() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 1)

		// The tick comes. New invocation is queued.
		m.now = m.now.Add(4*time.Second + 725980746*time.Nanosecond)
		m.roll(func(sm *StateMachine) { sm.OnTimerTick(1) })
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.state.TickNonce, ShouldEqual, 0)
		So(m.actions, ShouldResemble, []Action{
			StartInvocationAction{InvocationNonce: 2},
		})
		m.actions = nil

		// Aborting the job moves it back to scheduled state and schedules a tick.
		m.roll(func(sm *StateMachine) { sm.OnManualAbort() })
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 3)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(9*time.Second + 725980746*time.Nanosecond), 3},
		})
		m.actions = nil
	})
}

type testStateMachine struct {
	state    JobState
	now      time.Time
	nonce    int64
	schedule *schedule.Schedule
	actions  []Action
}

func newTestStateMachine(scheduleExpr string) *testStateMachine {
	// Each 5 sec.
	sched, _ := schedule.Parse(scheduleExpr, 0)
	return &testStateMachine{
		state: JobState{
			State: JobStateDisabled,
		},
		now:      epoch,
		schedule: sched,
	}
}

func (t *testStateMachine) rollWithErr(cb func(sm *StateMachine) error) error {
	nonce := t.nonce
	sm := StateMachine{
		State:    t.state,
		Now:      t.now,
		Schedule: t.schedule,
		Nonce: func() int64 {
			nonce++
			return nonce
		},
	}
	if err := cb(&sm); err != nil {
		return err
	}
	t.state = sm.State
	t.nonce = nonce
	t.actions = append(t.actions, sm.Actions...)
	return nil
}

func (t *testStateMachine) roll(cb func(sm *StateMachine)) {
	t.rollWithErr(func(sm *StateMachine) error {
		cb(sm)
		return nil
	})
}
