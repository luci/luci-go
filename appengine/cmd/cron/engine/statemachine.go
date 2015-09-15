// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package engine

import (
	"fmt"
	"time"
)

// StateKind defines high-level state of the job. See JobState for full state
// description: it's a StateKind plus additional parameters.
type StateKind string

const (
	// JobStateDisabled means the job is disabled or have never been seen before.
	// This is the initial state.
	JobStateDisabled StateKind = "DISABLED"

	// JobStateScheduled means the job is scheduled to start sometime in
	// the future and previous invocation is NOT running currently.
	JobStateScheduled StateKind = "SCHEDULED"

	// JobStateQueued means the job's invocation has been added to the task queue
	// and the job should start (switch to RUNNING state) really soon now.
	JobStateQueued StateKind = "QUEUED"

	// JobStateRunning means the job's invocation is running currently and the job
	// is scheduled to start again sometime in the future.
	JobStateRunning StateKind = "RUNNING"

	// JobStateOverrun means the job's new invocation should have been started
	// by now, but the previous one is still running.
	JobStateOverrun StateKind = "OVERRUN"

	// JobStateSlowQueue means the job's new invocation should have been started
	// by now, but the previous one is still sitting in the start queue.
	JobStateSlowQueue StateKind = "SLOW_QUEUE"
)

// JobState contains the current state of a job state machine.
type JobState struct {
	State          StateKind // the overall state of the job
	TickID         int64     // id of the expected OnTimerTick event
	TickTime       time.Time // when the OnTimerTick event is expected
	InvocationID   int64     // id of the currently queued or running invocation
	InvocationTime time.Time // when invocation was started
	Overruns       int       // how many times current invocation overran
}

// Action is a particular action to perform when switching the state. Can be
// type cast to some concrete *Action struct.
type Action interface {
	IsAction() bool
}

// TickLaterAction schedules an OnTimerTick(tickID) call at give moment in
// time (or close to it). TickID is used to skip canceled or repeated ticks.
type TickLaterAction struct {
	When   time.Time
	TickID int64
}

// IsAction makes TickLaterAction implement Action interface.
func (a TickLaterAction) IsAction() bool { return true }

// StartInvocationAction enqueues invocation of the actual job.
// OnInvocationDone(invocationID) will be called sometime later when the job
// is done.
type StartInvocationAction struct {
	InvocationID int64
}

// IsAction makes StartInvocationAction implement Action interface.
func (a StartInvocationAction) IsAction() bool { return true }

// StateMachine advances state of some single cron job. It performs a single
// step only (one On* call). As input it takes the state of the job and state of
// the world (the schedule is considered to be a part of the world state).
// As output it produces a new state and a set of actions. Handler of the state
// machine must guarantee that if state change was committed, all actions will
// be executed at least once.
//
// On* methods mutate OutputState or Actions and return errors only on transient
// conditions fixable by a retry. OutputState and Actions are mutated in place
// just to simplify APIs. Otherwise all On* transitions can be considered as
// pure functions (InputState, World) -> (OutputState, Actions).
//
// The lifecycle of a healthy cron job:
// DISABLED -> SCHEDULED -> QUEUED -> RUNNING -> SCHEDULED -> ...
type StateMachine struct {
	// Inputs.
	InputState         JobState     // job's current state
	Now                time.Time    // current time
	NextInvocationTime time.Time    // when to run the job next time
	Nonce              func() int64 // produces a series of nonces on demand

	// Outputs.
	OutputState *JobState // state after the transition or nil if same as input
	Actions     []Action  // emitted actions
}

// OnJobEnabled happens when a new job (never seen before) was discovered or
// a previously disabled job was enabled.
func (m *StateMachine) OnJobEnabled() error {
	if m.InputState.State == JobStateDisabled {
		m.schedule(JobStateScheduled)
	}
	return nil
}

// OnJobDisabled happens when job was disabled or removed. It clears the state
// and cancels any pending invocations (running ones continue to run though).
func (m *StateMachine) OnJobDisabled() error {
	m.OutputState = &JobState{State: JobStateDisabled}
	return nil
}

// OnTimerTick happens when scheduled timer (added with TickLaterAction) ticks.
func (m *StateMachine) OnTimerTick(tickID int64) error {
	// Skip unexpected, late or canceled ticks.
	if !m.expectingTick() || m.InputState.TickID != tickID {
		return nil
	}

	// Report transient error (to trigger retry) if the tick happened unexpectedly
	// soon.
	//
	// TODO(vadimsh): Do something with huge delays? Skip very late jobs?
	delay := m.Now.Sub(m.InputState.TickTime)
	if delay < 0 {
		return fmt.Errorf("tick happened %.1f sec before it was expected", -delay.Seconds())
	}

	// Was waiting for a tick to start a job? Add invocation to the queue.
	if m.InputState.State == JobStateScheduled {
		m.schedule(JobStateQueued)
		m.OutputState.InvocationTime = m.Now
		m.OutputState.InvocationID = m.Nonce()
		m.emitAction(StartInvocationAction{m.OutputState.InvocationID})
		return nil
	}

	// Already running a job (or have one in the queue) and it's time to launch
	// a new invocation? Skip this tick completely (schedule the next tick, carry
	// over invocation state), we do not want two copies of a job running.
	//
	// TODO(vadimsh): Make overrun policy configurable. Also handle permanently
	// stuck jobs.
	switch m.InputState.State {
	case JobStateRunning, JobStateOverrun:
		m.schedule(JobStateOverrun)
	case JobStateQueued, JobStateSlowQueue:
		m.schedule(JobStateSlowQueue)
	default:
		panic("Impossible, see expectingTick()")
	}
	m.OutputState.InvocationTime = m.InputState.InvocationTime
	m.OutputState.InvocationID = m.InputState.InvocationID
	m.OutputState.Overruns = m.InputState.Overruns + 1
	return nil
}

// OnInvocationStart happens when enqueued invocation finally starts to run.
func (m *StateMachine) OnInvocationStart(invocationID int64) error {
	s := m.InputState.State
	if s != JobStateQueued && s != JobStateSlowQueue {
		return nil
	}
	if m.InputState.InvocationID != invocationID {
		return nil
	}
	cp := m.InputState
	m.OutputState = &cp
	if s == JobStateQueued {
		m.OutputState.State = JobStateRunning
	} else { // JobStateSlowQueue
		m.OutputState.State = JobStateOverrun
	}
	return nil
}

// OnInvocationDone happens when invocation completes.
func (m *StateMachine) OnInvocationDone(invocationID int64) error {
	// Ignore unexpected events. Can happen if job was moved to disabled state
	// while invocation was still running.
	if m.InputState.State != JobStateRunning && m.InputState.State != JobStateOverrun {
		return nil
	}
	if m.InputState.InvocationID != invocationID {
		return nil
	}
	// Do not schedule another tick. It was already scheduled when job entered
	// JobStateQueued state.
	// TODO(vadimsh): Start the job right away if overrun happened? Retry job on
	// failure?
	m.OutputState = &JobState{
		State:    JobStateScheduled,
		TickTime: m.InputState.TickTime,
		TickID:   m.InputState.TickID,
	}
	return nil
}

// OnScheduleChange happens when job's schedule changes (and the job potentially
// needs to be rescheduled).
func (m *StateMachine) OnScheduleChange() error {
	// Did it really change next invocation time?
	if !m.expectingTick() || m.NextInvocationTime == m.InputState.TickTime {
		return nil
	}
	// Change the next tick time, carry over the rest of the state.
	cp := m.InputState
	m.OutputState = &cp
	m.OutputState.TickTime = m.NextInvocationTime
	m.OutputState.TickID = m.Nonce()
	m.emitAction(TickLaterAction{m.OutputState.TickTime, m.OutputState.TickID})
	return nil
}

////////////////////////////////////////////////////////////////////////////////

// expectingTick returns true if current state implies that machine is waiting
// for a timer tick.
func (m *StateMachine) expectingTick() bool {
	switch m.InputState.State {
	case JobStateScheduled, JobStateQueued, JobStateRunning, JobStateOverrun, JobStateSlowQueue:
		return true
	}
	return false
}

// schedule emits TickLaterAction action according to job's schedule and
// generates OutputState putting tick time and tick ID there. It does NOT
// preserve invocation ID and related fields from InputState. Copy them from
// InputState if needed.
func (m *StateMachine) schedule(s StateKind) {
	m.OutputState = &JobState{
		State:    s,
		TickTime: m.NextInvocationTime,
		TickID:   m.Nonce(),
	}
	m.emitAction(TickLaterAction{m.OutputState.TickTime, m.OutputState.TickID})
}

// emitAction adds an action to 'actions' array.
func (m *StateMachine) emitAction(a Action) {
	m.Actions = append(m.Actions, a)
}
