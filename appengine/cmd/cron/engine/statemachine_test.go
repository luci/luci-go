// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package engine

import (
	"testing"
	"time"

	"github.com/gorhill/cronexpr"
	. "github.com/smartystreets/goconvey/convey"
)

var epoch = time.Unix(1442270520, 0).UTC()

func TestStateMachine(t *testing.T) {
	Convey("Normal flow", t, func() {
		m := newTestStateMachine()

		// Noop when in disabled state.
		So(m.roll(func(sm *StateMachine) error { return sm.OnTimerTick(1) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateDisabled)
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationStart(1) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateDisabled)
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationDone(1) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateDisabled)
		So(m.roll(func(sm *StateMachine) error { return sm.OnScheduleChange() }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateDisabled)

		// Enabling schedules a tick.
		So(m.roll(func(sm *StateMachine) error { return sm.OnJobEnabled() }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(5 * time.Second), 1},
		})
		m.actions = nil

		// Re enabling does nothing.
		So(m.roll(func(sm *StateMachine) error { return sm.OnJobEnabled() }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldBeNil)

		// Wrong tick ID is skipped.
		So(m.roll(func(sm *StateMachine) error { return sm.OnTimerTick(2) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldBeNil)

		// Early tick causes retry.
		So(m.roll(func(sm *StateMachine) error { return sm.OnTimerTick(1) }), ShouldNotBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldBeNil)

		// Tick on time moves to JobStateQueued, schedules next tick.
		m.now = m.now.Add(5 * time.Second)
		So(m.roll(func(sm *StateMachine) error { return sm.OnTimerTick(1) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(10 * time.Second), 2},
			StartInvocationAction{3},
		})
		m.actions = nil

		// Wrong invocation ID is skipped.
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationStart(333) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.actions, ShouldBeNil)

		// Time to run.
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationStart(3) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateRunning)

		// Skip wrong invocation ID.
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationDone(333) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateRunning)

		// End of the cycle. Ends up in scheduled state, waiting for the tick added
		// when StartInvocationAction was issued.
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationDone(3) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 2)

		// Disable cancels the timer.
		So(m.roll(func(sm *StateMachine) error { return sm.OnJobDisabled() }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateDisabled)
		So(m.state.InvocationNonce, ShouldEqual, 0)
	})

	Convey("Overrun when queued", t, func() {
		m := newTestStateMachine()

		// Enabling schedules a tick.
		So(m.roll(func(sm *StateMachine) error { return sm.OnJobEnabled() }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(5 * time.Second), 1},
		})
		m.actions = nil

		// Tick on time moves to JobStateQueued
		m.now = m.now.Add(5 * time.Second)
		So(m.roll(func(sm *StateMachine) error { return sm.OnTimerTick(1) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(10 * time.Second), 2},
			StartInvocationAction{3},
		})
		m.actions = nil

		// Overrun when queued.
		m.now = m.now.Add(5 * time.Second)
		So(m.roll(func(sm *StateMachine) error { return sm.OnTimerTick(2) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateSlowQueue)
		So(m.state.Overruns, ShouldEqual, 1)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(15 * time.Second), 4},
		})
		m.actions = nil

		// Time to run. Moves to JobStateOverrun because was stuck in queue.
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationStart(3) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateOverrun)
		So(m.state.Overruns, ShouldEqual, 1)

		// End of the cycle.
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationDone(3) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
	})

	Convey("Overrun when running", t, func() {
		m := newTestStateMachine()

		// Enabling schedules a tick.
		So(m.roll(func(sm *StateMachine) error { return sm.OnJobEnabled() }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(5 * time.Second), 1},
		})
		m.actions = nil

		// Tick on time moves to JobStateQueued
		m.now = m.now.Add(5 * time.Second)
		So(m.roll(func(sm *StateMachine) error { return sm.OnTimerTick(1) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateQueued)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(10 * time.Second), 2},
			StartInvocationAction{3},
		})
		m.actions = nil

		// Time to run.
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationStart(3) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateRunning)

		// Next tick comes while job is running.
		m.now = m.now.Add(5 * time.Second)
		So(m.roll(func(sm *StateMachine) error { return sm.OnTimerTick(2) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateOverrun)
		So(m.state.Overruns, ShouldEqual, 1)
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(15 * time.Second), 4},
		})
		m.actions = nil

		// End of the cycle.
		So(m.roll(func(sm *StateMachine) error { return sm.OnInvocationDone(3) }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
	})

	Convey("Schedule changes", t, func() {
		m := newTestStateMachine()

		// Enabling schedules a tick after 5 sec.
		So(m.roll(func(sm *StateMachine) error { return sm.OnJobEnabled() }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 1)
		So(m.state.TickTime, ShouldResemble, epoch.Add(5*time.Second))
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(5 * time.Second), 1},
		})
		m.actions = nil

		// Rescheduling event with same NextInvocationTime time is noop.
		So(m.roll(func(sm *StateMachine) error { return sm.OnScheduleChange() }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 1)
		So(m.state.TickTime, ShouldResemble, epoch.Add(5*time.Second))
		So(m.actions, ShouldBeNil)

		// Suddenly ticking each second.
		expr, _ := cronexpr.Parse("*/1 * * * * * *")
		m.schedule = expr

		// Should be rescheduled to run earlier
		So(m.roll(func(sm *StateMachine) error { return sm.OnScheduleChange() }), ShouldBeNil)
		So(m.state.State, ShouldEqual, JobStateScheduled)
		So(m.state.TickNonce, ShouldEqual, 2)
		So(m.state.TickTime, ShouldResemble, epoch.Add(1*time.Second))
		So(m.actions, ShouldResemble, []Action{
			TickLaterAction{epoch.Add(1 * time.Second), 2},
		})
	})
}

type testStateMachine struct {
	state    JobState
	now      time.Time
	nonce    int64
	schedule *cronexpr.Expression
	actions  []Action
}

func newTestStateMachine() *testStateMachine {
	// Each 5 sec.
	expr, _ := cronexpr.Parse("*/5 * * * * * *")
	return &testStateMachine{
		state: JobState{
			State: JobStateDisabled,
		},
		now:      epoch,
		schedule: expr,
	}
}

func (t *testStateMachine) roll(cb func(sm *StateMachine) error) error {
	nonce := t.nonce
	sm := StateMachine{
		InputState:         t.state,
		Now:                t.now,
		NextInvocationTime: t.schedule.Next(t.now),
		Nonce: func() int64 {
			nonce++
			return nonce
		},
	}
	if err := cb(&sm); err != nil {
		return err
	}
	if sm.OutputState != nil {
		t.state = *sm.OutputState
	}
	t.nonce = nonce
	t.actions = append(t.actions, sm.Actions...)
	return nil
}
