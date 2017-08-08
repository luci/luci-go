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

package engine

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/scheduler/appengine/schedule"
	"go.chromium.org/luci/server/auth/identity"
)

// StateKind defines high-level state of the job. See JobState for full state
// description: it's a StateKind plus additional parameters.
//
// Note that this is low-level state that is not directly visible in the UI. For
// UI we derive more user-friendly state labels by examining JobState and taking
// into account job traits. See makeJob in ui/presentation.go.
type StateKind string

const (
	// JobStateDisabled means the job is disabled or have never been seen before.
	// This is the initial state.
	JobStateDisabled StateKind = "DISABLED"

	// JobStateScheduled means the job is scheduled to start sometime in
	// the future and previous invocation is NOT running currently.
	JobStateScheduled StateKind = "SCHEDULED"

	// JobStateSuspended means the job is not running now, and no ticks are
	// scheduled. It's used for jobs on "triggered" schedule or for paused jobs.
	//
	// Technically SUSPENDED is like SCHEDULED with the tick in the distant
	// future.
	JobStateSuspended StateKind = "SUSPENDED"

	// JobStateQueued means the job's invocation has been added to the task queue
	// and the job should start (switch to "RUNNING" state inside TaskManager's
	// LaunchTask call) really soon now.
	//
	// The engine will keep trying to launch the queued invocation as long as
	// the job is in "QUEUED" state (e.g. it makes another launch attempt if
	// LaunchTask crashes). But once the job is in "RUNNING" state, it's the
	// responsibility of the particular TaskManager implementation to ensure the
	// invocation state gets updated in a timely manner.
	//
	// Some TaskManagers may opt to skip "RUNNING" state completely and do
	// everything in one go in LaunchTask (by switching invocation to some final
	// state in the end). This is allowed. If such task crashes before completion,
	// it will be retried by the engine (because it is in "QUEUED" state).
	JobStateQueued StateKind = "QUEUED"

	// JobStateRunning means the job's invocation is running currently and the job
	// is scheduled to start again sometime in the future.
	JobStateRunning StateKind = "RUNNING"

	// JobStateOverrun is same as "RUNNING", except the engine has also detected
	// an overrun: the job's new invocation should have been started by now, but
	// the previous one is still running.
	JobStateOverrun StateKind = "OVERRUN"

	// JobStateSlowQueue is same as "QUEUED", except the engine has also detected
	// an overrun: the job's new invocation should have been started by now, but
	// the previous one is still sitting in the queue.
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

	// PrevTime is when last invocation has finished (successfully or not).
	PrevTime time.Time `gae:",noindex"`

	// InvocationNonce is id of the currently queued or running invocation
	// request. Once it is processed, it produces some new invocation identified
	// by InvocationID. Single InvocationNonce can spawn many InvocationIDs due to
	// retries on transient errors when starting an invocation. Only one of them
	// will end up in Running state though.
	InvocationNonce int64 `gae:",noindex"`

	// InvocationRetryCount is how many times an invocation request (identified by
	// InvocationNonce) was attempted to start and failed.
	InvocationRetryCount int `gae:",noindex"`

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

// IsRetrying is true if there some invocation queued that is a retry of
// a previously attempted invocation.
//
// An invocation is retried when it fails to transition out from STARTING state
// (e.g. crashes before switching to RUNNING or any of the final states).
func (s *JobState) IsRetrying() bool {
	return (s.State == JobStateQueued || s.State == JobStateSlowQueue) && s.InvocationRetryCount != 0
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

// StateMachine advances state of some single scheduler job. It performs
// a single step only (one On* call). As input it takes the state of the job and
// state of the world (the schedule is considered to be a part of the world
// state). As output it produces a new state and a set of actions. Handler of
// the state machine must guarantee that if state change was committed, all
// actions will be executed at least once.
//
// On* methods mutate State or Actions and return errors only on transient
// conditions fixable by a retry. State and Actions are mutated in place
// just to simplify APIs. Otherwise all On* transitions can be considered as
// pure functions (State, World) -> (State, Actions).
//
// The lifecycle of a healthy job:
// DISABLED -> SCHEDULED -> QUEUED -> QUEUED (starting) -> RUNNING -> SCHEDULED
type StateMachine struct {
	// Inputs.
	Now      time.Time          // current time
	Schedule *schedule.Schedule // knows when to run the job next time
	Nonce    func() int64       // produces a series of nonces on demand

	// Mutated.
	State   JobState // state of the job, mutated in On* methods
	Actions []Action // emitted actions

	// For adhoc logging when debugging locally.
	Context context.Context
}

// OnJobEnabled happens when a new job (never seen before) was discovered or
// a previously disabled job was enabled.
func (m *StateMachine) OnJobEnabled() {
	if m.State.State == JobStateDisabled {
		m.State = JobState{State: JobStateScheduled} // clean state
		m.scheduleTick()
		m.maybeSuspendOrResume()
	}
}

// OnJobDisabled happens when job was disabled or removed. It clears the state
// and cancels any pending invocations (running ones continue to run though).
func (m *StateMachine) OnJobDisabled() {
	m.State = JobState{State: JobStateDisabled} // clean state
}

// OnTimerTick happens when scheduled timer (added with TickLaterAction) ticks.
func (m *StateMachine) OnTimerTick(tickNonce int64) error {
	// Skip unexpected, late or canceled ticks.
	switch {
	case m.State.State == JobStateDisabled:
		return nil
	case m.State.State == JobStateSuspended:
		return nil
	case m.State.TickNonce != tickNonce:
		return nil
	}

	// Report error (to trigger retry) if the tick happened unexpectedly soon.
	// Absolute schedules may report "wrong" next tick time if asked for a next
	// tick before previous one has happened.
	delay := m.Now.Sub(m.State.TickTime)
	if delay < 0 {
		return fmt.Errorf("tick happened %.1f sec before it was expected", -delay.Seconds())
	}

	// Start waiting for a new tick right away if on an absolute schedule (or just
	// clear the tick state if on a relative one, new tick will be set when
	// invocation completes, see OnInvocationDone).
	if m.Schedule.IsAbsolute() {
		m.scheduleTick()
	} else {
		m.resetTick()
	}

	// Was waiting for a tick to start a job? Add invocation to the queue.
	if m.State.State == JobStateScheduled {
		m.State.State = JobStateQueued
		m.queueInvocation("")
		return nil
	}

	// Already running a job (or have one in the queue) and it's time to launch
	// a new invocation? Skip this tick completely.
	//
	// TODO(vadimsh): Make overrun policy configurable. Also handle permanently
	// stuck jobs.
	switch m.State.State {
	case JobStateRunning, JobStateOverrun:
		m.State.State = JobStateOverrun
	case JobStateQueued, JobStateSlowQueue:
		m.State.State = JobStateSlowQueue
	default:
		impossible("impossible state %s", m.State.State)
	}
	m.State.Overruns++
	m.emitAction(RecordOverrunAction{
		Overruns:            m.State.Overruns,
		RunningInvocationID: m.State.InvocationID,
	})
	return nil
}

// OnInvocationStarting happens when the engine picks up enqueued invocation and
// attempts to start it. Engine calls OnInvocationStarted when it succeeds at
// launching the invocation. Engine calls OnInvocationStarting again with
// another invocationID if previous launch attempt failed.
func (m *StateMachine) OnInvocationStarting(invocationNonce, invocationID int64, retryCount int) {
	if m.State.IsExpectingInvocation(invocationNonce) {
		m.State.InvocationID = invocationID
		m.State.InvocationRetryCount = retryCount
	}
}

// OnInvocationStarted happens when enqueued invocation finally starts to run.
func (m *StateMachine) OnInvocationStarted(invocationID int64) {
	if m.State.InvocationID == invocationID {
		switch m.State.State {
		case JobStateQueued:
			m.State.State = JobStateRunning
		case JobStateSlowQueue:
			m.State.State = JobStateOverrun
		default:
			impossible("impossible state %s", m.State.State)
		}
	}
}

// OnInvocationDone happens when invocation completes.
func (m *StateMachine) OnInvocationDone(invocationID int64) {
	// Ignore unexpected events. Can happen if job was moved to disabled state
	// while invocation was still running.
	if m.State.State != JobStateRunning && m.State.State != JobStateOverrun {
		return
	}
	if m.State.InvocationID != invocationID {
		return
	}
	m.invocationFinished()
}

// OnScheduleChange happens when job's schedule changes (and the job potentially
// needs to be rescheduled).
func (m *StateMachine) OnScheduleChange() {
	// Do not touch timers on disabled jobs.
	if m.State.State == JobStateDisabled {
		return
	}

	// If the job was running on a relative schedule (and thus has no pending
	// ticks), and the new schedule is absolute, we'd need to schedule a new tick
	// now. If the new schedule is also relative, do nothing: it will be used as
	// usual when currently running invocation finishes.
	if m.State.TickTime.IsZero() {
		if m.Schedule.IsAbsolute() {
			m.scheduleTick()
		}
		return
	}

	// When switching from an absolute to a relative schedule, cancel pending
	// tick, so that the new schedule is used when the current invocation ends.
	// Don't do it if the job is waiting to start a new invocation: we'd need to
	// move the tick in this case, not cancel it. This situation is handled below.
	isWaiting := m.State.State == JobStateScheduled || m.State.State == JobStateSuspended
	if !m.Schedule.IsAbsolute() && !isWaiting {
		m.resetTick()
		return
	}

	// At this point we know that the job must have a tick enabled, because
	// either it's running on an absolute schedule (such jobs always "tick",
	// regardless of the state), or it's running on a relative schedule, and
	// currently it's in between invocations (in Scheduled state). Reschedule
	// the tick if it changed.
	m.scheduleTick()
	m.maybeSuspendOrResume()
}

// OnManualInvocation happens when user starts invocation via "Run now" button.
// Manual invocation only works if the job is currently not running or not
// queued for run (i.e. it is in Scheduled state waiting for a timer tick).
func (m *StateMachine) OnManualInvocation(triggeredBy identity.Identity) error {
	if m.State.State != JobStateScheduled && m.State.State != JobStateSuspended {
		return errors.New("the job is already running or about to start")
	}
	m.State.State = JobStateQueued
	m.queueInvocation(triggeredBy)
	if !m.Schedule.IsAbsolute() {
		m.resetTick() // will be set again when invocation ends
	}
	return nil
}

// OnManualAbort happens when users aborts the queued or running invocation.
func (m *StateMachine) OnManualAbort() {
	// Pretend that it is finished. InvocationNonce is not 0 only if an invocation
	// is running or queued.
	if m.State.InvocationNonce != 0 {
		m.invocationFinished()
	}
}

////////////////////////////////////////////////////////////////////////////////

// scheduleTick emits TickLaterAction action according to job's schedule. Does
// nothing if the tick is already scheduled.
func (m *StateMachine) scheduleTick() {
	nextTick := m.Schedule.Next(m.Now, m.State.PrevTime)
	if nextTick != m.State.TickTime {
		m.State.TickTime = nextTick
		m.State.TickNonce = m.Nonce()
		if nextTick != schedule.DistantFuture {
			m.emitAction(TickLaterAction{
				When:      m.State.TickTime,
				TickNonce: m.State.TickNonce,
			})
		}
	}
}

// invocationFinished is called after invocation is finished to return the
// job to scheduled (or suspended) state and emit a timer tick (if necessary).
func (m *StateMachine) invocationFinished() {
	m.State.State = JobStateScheduled
	m.State.PrevTime = m.Now
	m.resetInvocation()      // forget about just finished invocation
	m.scheduleTick()         // start waiting for a new one
	m.maybeSuspendOrResume() // switch back to suspended state if necessary
}

// maybeSuspendOrResume switches SCHEDULED state to SUSPENDED state in case
// current tick is scheduled for DistantFuture, or switches SUSPENDED to
// SCHEDULED if current tick is not scheduled for DistantFuture.
func (m *StateMachine) maybeSuspendOrResume() {
	switch {
	case m.State.State == JobStateScheduled && m.State.TickTime == schedule.DistantFuture:
		m.State.State = JobStateSuspended
	case m.State.State == JobStateSuspended && m.State.TickTime != schedule.DistantFuture:
		m.State.State = JobStateScheduled
	}
}

// resetTick clears tick time and nonce, effectively canceling a tick.
func (m *StateMachine) resetTick() {
	m.State.TickTime = time.Time{}
	m.State.TickNonce = 0
}

// queueInvocation generates a new invocation nonce and asks engine to start
// a new invocation.
func (m *StateMachine) queueInvocation(triggeredBy identity.Identity) {
	m.State.InvocationTime = m.Now
	m.State.InvocationNonce = m.Nonce()
	m.State.InvocationRetryCount = 0
	m.State.InvocationID = 0
	m.State.Overruns = 0
	m.emitAction(StartInvocationAction{
		InvocationNonce: m.State.InvocationNonce,
		TriggeredBy:     triggeredBy,
	})
}

// resetInvocation clears invocation related part of the state.
func (m *StateMachine) resetInvocation() {
	m.State.InvocationNonce = 0
	m.State.InvocationRetryCount = 0
	m.State.InvocationTime = time.Time{}
	m.State.InvocationID = 0
	m.State.Overruns = 0
}

// emitAction adds a generic action to 'actions' array.
func (m *StateMachine) emitAction(a Action) {
	m.Actions = append(m.Actions, a)
}

// impossible is never actually called.
func impossible(msg string, args ...interface{}) {
	panic(fmt.Errorf(msg, args...))
}
