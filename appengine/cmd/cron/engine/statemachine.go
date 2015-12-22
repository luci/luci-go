// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package engine

import (
	"errors"
	"fmt"
	"time"

	"github.com/luci/luci-go/appengine/cmd/cron/schedule"
	"github.com/luci/luci-go/server/auth/identity"
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
	// State is the overall state of the job (like "RUNNING" or "DISABLED", etc).
	State StateKind

	// Overruns is how many times current invocation overran.
	Overruns int

	// TickNonce is id of the next expected OnTimerTick event.
	TickNonce int64 `gae:",noindex"`

	// TickTime is when the OnTimerTick event is expected.
	TickTime time.Time `gae:",noindex"`

	// InvocationNonce is id of the currently queued or running invocation
	// request. Once it is processed, it produces some new invocation identified
	// by InvocationID. Single InvocationNonce can spawn many InvocationIDs due to
	// retries on transient errors when starting an invocation. Only one of them
	// will end up in Running state though.
	InvocationNonce int64 `gae:",noindex"`

	// InvocationTime is when the current invocation request was queued.
	InvocationTime time.Time `gae:",noindex"`

	// InvocationID is ID of currently running invocation or 0 if none is running.
	InvocationID int64 `gae:",noindex"`
}

// IsExpectingInvocation returns true if the state machine accepts
// OnInvocationStarting event with given nonce.
func (s *JobState) IsExpectingInvocation(invocationNonce int64) bool {
	return (s.State == JobStateQueued || s.State == JobStateSlowQueue) && s.InvocationNonce == invocationNonce
}

// IsExpectingTick returns true if the state implies that the state machine is
// waiting for a timer tick.
func (s *JobState) IsExpectingTick() bool {
	switch s.State {
	case JobStateScheduled, JobStateQueued, JobStateRunning, JobStateOverrun, JobStateSlowQueue:
		return true
	}
	return false
}

// Action is a particular action to perform when switching the state. Can be
// type cast to some concrete *Action struct.
type Action interface {
	IsAction() bool
}

// TickLaterAction schedules an OnTimerTick(tickNonce) call at give moment in
// time (or close to it). TickNonce is used to skip canceled or repeated ticks.
type TickLaterAction struct {
	When      time.Time
	TickNonce int64
}

// IsAction makes TickLaterAction implement Action interface.
func (a TickLaterAction) IsAction() bool { return true }

// StartInvocationAction enqueues invocation of the actual job.
// OnInvocationDone(invocationNonce) will be called sometime later when the job
// is done.
type StartInvocationAction struct {
	InvocationNonce int64
	TriggeredBy     identity.Identity
}

// IsAction makes StartInvocationAction implement Action interface.
func (a StartInvocationAction) IsAction() bool { return true }

// RecordOverrunAction instructs Engine to record overrun event.
//
// An overrun happens when job's schedule indicates that a new job invocation
// should start now, but previous one is still running.
type RecordOverrunAction struct {
	Overruns            int
	RunningInvocationID int64
}

// IsAction makes RecordOverrunAction implement Action interface.
func (a RecordOverrunAction) IsAction() bool { return true }

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
// DISABLED -> SCHEDULED -> QUEUED -> QUEUED (starting) -> RUNNING -> SCHEDULED
type StateMachine struct {
	// Inputs.
	InputState JobState           // job's current state
	Now        time.Time          // current time
	Schedule   *schedule.Schedule // knows when to run the job next time
	Nonce      func() int64       // produces a series of nonces on demand

	// Outputs.
	OutputState *JobState // state after the transition or nil if same as input
	Actions     []Action  // emitted actions
}

// OnJobEnabled happens when a new job (never seen before) was discovered or
// a previously disabled job was enabled.
func (m *StateMachine) OnJobEnabled() error {
	if m.InputState.State == JobStateDisabled {
		m.schedule(JobStateScheduled, JobState{})
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
func (m *StateMachine) OnTimerTick(tickNonce int64) error {
	// Skip unexpected, late or canceled ticks.
	if !m.InputState.IsExpectingTick() || m.InputState.TickNonce != tickNonce {
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
		m.schedule(JobStateQueued, JobState{})
		m.OutputState.InvocationTime = m.Now
		m.OutputState.InvocationNonce = m.Nonce()
		m.emitAction(StartInvocationAction{
			InvocationNonce: m.OutputState.InvocationNonce,
		})
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
		m.schedule(JobStateOverrun, m.InputState)
	case JobStateQueued, JobStateSlowQueue:
		m.schedule(JobStateSlowQueue, m.InputState)
	default:
		panic("Impossible, see IsExpectingTick()")
	}
	m.OutputState.Overruns++
	m.emitAction(RecordOverrunAction{
		Overruns:            m.OutputState.Overruns,
		RunningInvocationID: m.InputState.InvocationID,
	})
	return nil
}

// OnInvocationStarting happens when the engine picks up enqueued invocation and
// attempts to start it. Engine calls OnInvocationStarted when it succeeds at
// launching the invocation. Engine calls OnInvocationStarting again with
// another invocationID if previous launch attempt failed.
func (m *StateMachine) OnInvocationStarting(invocationNonce, invocationID int64) error {
	if m.InputState.IsExpectingInvocation(invocationNonce) {
		cp := m.InputState
		m.OutputState = &cp
		m.OutputState.InvocationID = invocationID
	}
	return nil
}

// OnInvocationStarted happens when enqueued invocation finally starts to run.
func (m *StateMachine) OnInvocationStarted(invocationID int64) error {
	if m.InputState.InvocationID == invocationID {
		cp := m.InputState
		m.OutputState = &cp
		if m.InputState.State == JobStateQueued {
			m.OutputState.State = JobStateRunning
		} else { // JobStateSlowQueue
			m.OutputState.State = JobStateOverrun
		}
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
		State:     JobStateScheduled,
		TickTime:  m.InputState.TickTime,
		TickNonce: m.InputState.TickNonce,
	}
	return nil
}

// OnScheduleChange happens when job's schedule changes (and the job potentially
// needs to be rescheduled).
func (m *StateMachine) OnScheduleChange() error {
	// Not scheduled?
	if !m.InputState.IsExpectingTick() {
		return nil
	}
	// Did it really change next invocation time?
	nextTick := m.Schedule.Next(m.Now)
	if nextTick == m.InputState.TickTime {
		return nil
	}
	// Change the next tick time, carry over the rest of the state.
	cp := m.InputState
	m.OutputState = &cp
	m.OutputState.TickTime = nextTick
	m.OutputState.TickNonce = m.Nonce()
	m.emitAction(TickLaterAction{m.OutputState.TickTime, m.OutputState.TickNonce})
	return nil
}

// OnManualInvocation happens when user starts invocation via "Run now" button.
// Manual invocation only works if the job is currently not running or not
// queued for run (i.e. it is in Scheduled state waiting for a timer tick).
func (m *StateMachine) OnManualInvocation(triggeredBy identity.Identity) error {
	if m.InputState.State != JobStateScheduled {
		return errors.New("the job is already running or about to start")
	}
	m.schedule(JobStateQueued, JobState{})
	m.OutputState.InvocationTime = m.Now
	m.OutputState.InvocationNonce = m.Nonce()
	m.emitAction(StartInvocationAction{
		InvocationNonce: m.OutputState.InvocationNonce,
		TriggeredBy:     triggeredBy,
	})
	return nil
}

////////////////////////////////////////////////////////////////////////////////

// schedule emits TickLaterAction action according to job's schedule and
// generates OutputState putting tick time and tick ID there. It does NOT
// preserve invocation ID and related fields from InputState. Copy them from
// InputState if needed.
func (m *StateMachine) schedule(s StateKind, prev JobState) {
	m.OutputState = &prev
	m.OutputState.State = s
	m.OutputState.TickTime = m.Schedule.Next(m.Now)
	m.OutputState.TickNonce = m.Nonce()
	m.emitAction(TickLaterAction{m.OutputState.TickTime, m.OutputState.TickNonce})
}

// emitAction adds an action to 'actions' array.
func (m *StateMachine) emitAction(a Action) {
	m.Actions = append(m.Actions, a)
}
