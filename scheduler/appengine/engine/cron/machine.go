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
	"fmt"
	"time"

	"go.chromium.org/luci/scheduler/appengine/schedule"
)

// State stores serializable state of the cron machine.
//
// Whoever hosts the cron machine is supposed to store this state in some
// persistent store between events. It's mutated by Machine. So the usage
// pattern is:
//   * Deserialize State, construct Machine instance with it.
//   * Invoke some Machine method (e.g Enable()) to advance the state.
//   * Acknowledge all actions emitted by the machine (see Machine.Actions).
//   * Serialize the mutated state (available in Machine.State).
//
// If appropriate, all of the above should be done in a transaction.
//
// Machine assumes that whoever hosts it handles TickLaterAction with following
// semantics:
//   * A scheduled tick can't be "unscheduled".
//   * A scheduled tick may come more than one time.
//
// So the machine just ignores ticks it doesn't expect.
//
// It supports "absolute" and "relative" schedules, see 'schedule' package for
// definitions.
type State struct {
	// Enabled is true if the cron machine is running.
	//
	// A disabled cron machine ignores all events except 'Enable'.
	Enabled bool

	// Generation is increased each time state mutates.
	//
	// Monotonic, never resets. Should not be assumed sequential: some calls
	// mutate the state multiple times during one transition.
	//
	// Used to deduplicate StartInvocationAction in case of retries of cron state
	// transitions.
	Generation int64

	// LastRewind is a time when the cron machine was restarted last time.
	//
	// For relative schedules, it's a time RewindIfNecessary() was called. For
	// absolute schedules it is last time invocation happened (cron machines on
	// absolute schedules auto-rewind themselves).
	LastRewind time.Time

	// LastTick is last emitted tick request (or empty struct).
	//
	// It may be scheduled for "distant future" for paused cron machines.
	LastTick TickLaterAction
}

// Equal reports whether two structs are equal.
func (s *State) Equal(o *State) bool {
	return s.Enabled == o.Enabled &&
		s.Generation == o.Generation &&
		s.LastRewind.Equal(o.LastRewind) &&
		s.LastTick.Equal(&o.LastTick)
}

// IsSuspended returns true if the cron machine is not waiting for a tick.
//
// This happens for paused cron machines (they technically are scheduled for
// a tick in a distant future) and for cron machines on relative schedule that
// wait for 'RewindIfNecessary' to be called to start ticking again.
//
// A disabled cron machine is also considered suspended.
func (s *State) IsSuspended() bool {
	return !s.Enabled || s.LastTick.When.IsZero() || s.LastTick.When == schedule.DistantFuture
}

////////////////////////////////////////////////////////////////////////////////

// Action is a particular action to perform when switching the state.
//
// Can be type cast to some concrete *Action struct. Intended to be handled by
// whoever hosts the cron machine.
type Action interface {
	IsAction() bool
}

// TickLaterAction schedules an OnTimerTick call at given moment in time.
//
// TickNonce is used by cron machine to skip canceled or repeated ticks.
type TickLaterAction struct {
	When      time.Time
	TickNonce int64
}

// Equal reports whether two structs are equal.
func (a *TickLaterAction) Equal(o *TickLaterAction) bool {
	return a.TickNonce == o.TickNonce && a.When.Equal(o.When)
}

// IsAction makes TickLaterAction implement Action interface.
func (a TickLaterAction) IsAction() bool { return true }

// StartInvocationAction is emitted when the scheduled moment comes.
//
// A handler is expected to call RewindIfNecessary() at some later time to
// restart the cron machine if it's running on a relative schedule (e.g. "with
// 10 sec interval"). Cron machines on relative schedules are "one shot". They
// need to be rewound to start counting time again.
//
// Cron machines on absolute schedules (regular crons, like "at 12 AM every
// day") don't need rewinding, they'll start counting time until next invocation
// automatically. Calling RewindIfNecessary() for them won't hurt though, it
// will be noop.
type StartInvocationAction struct {
	Generation int64 // value of state.Generation when the action was emitted
}

// IsAction makes StartInvocationAction implement Action interface.
func (a StartInvocationAction) IsAction() bool { return true }

////////////////////////////////////////////////////////////////////////////////

// Machine advances the state of the cron machine.
//
// It gracefully handles various kinds of external events (like pauses and
// schedule changes) and emits actions that's supposed to handled by whoever
// hosts it.
type Machine struct {
	// Inputs.
	Now      time.Time          // current time
	Schedule *schedule.Schedule // knows when to emit invocation action
	Nonce    func() int64       // produces nonces on demand

	// Mutated.
	State   State    // state of the cron machine, mutated by its methods
	Actions []Action // all emitted actions (if any)
}

// Enable makes the cron machine start counting time.
//
// Does nothing if already enabled.
func (m *Machine) Enable() {
	if !m.State.Enabled {
		m.State = State{
			Enabled:    true,
			Generation: m.nextGen(),
			LastRewind: m.Now,
		}
		m.scheduleTick(m.Now)
	}
}

// Disable stops any pending timer ticks, resets state.
//
// The cron machine will ignore any events until Enable is called to turn it on.
func (m *Machine) Disable() {
	m.State = State{Enabled: false, Generation: m.nextGen()}
}

// RewindIfNecessary is called to restart the cron after it has fired the
// invocation action.
//
// Does nothing if the cron is disabled or already ticking.
func (m *Machine) RewindIfNecessary() {
	m.rewindIfNecessary(m.Now)
}

// OnScheduleChange happens when cron's schedule changes.
//
// In particular, it handles switches between absolute and relative schedules.
func (m *Machine) OnScheduleChange() {
	// Do not touch timers on disabled cron machines.
	if !m.State.Enabled {
		return
	}

	// The following condition is true for cron machines on a relative schedule
	// that have already "fired", and currently wait for manual RewindIfNecessary
	// call to start ticking again. When such cron machines switch to an absolute
	// schedule, we need to rewind them right away (since machines on absolute
	// schedules always tick!). If the new schedule is also relative, do nothing:
	// RewindIfNecessary() should be called manually by the host at some later
	// time (as usual for relative schedules).
	if m.State.LastTick.When.IsZero() {
		if m.Schedule.IsAbsolute() {
			m.RewindIfNecessary()
		}
	} else {
		// In this branch, the cron machine has a timer tick scheduled. It means it
		// is either in a relative or absolute schedule, and this schedule may have
		// changed, so we may need to move the tick to reflect the change. Note that
		// we are not resetting LastRewind here, since we want the new schedule to
		// take into account real last RewindIfNecessary call. For example, if the
		// last rewind happened at moment X, current time is Now, and the new
		// schedule is "with 10s interval", we want the tick to happen at "X+10",
		// not "Now+10".
		m.scheduleTick(m.Now)
	}
}

// OnTimerTick happens when a scheduled timer tick (added with TickLaterAction)
// occurs.
//
// Returns an error if the tick happened too soon.
func (m *Machine) OnTimerTick(tickNonce int64) error {
	// Silently skip unexpected, late or canceled ticks. This is fine.
	switch {
	case m.State.IsSuspended():
		return nil
	case m.State.LastTick.TickNonce != tickNonce:
		return nil
	}

	// Report error (to trigger a retry) if the tick happened unexpectedly soon.
	// Allow up to 50ms clock drift, but correct it to be 0. This is important for
	// getting the correct next tick time from an absolute schedule. If we pass
	// m.Now to the cron schedule uncorrected, we'll just get the time of the
	// already scheduled tick (the one we are handling now, since uncorrected
	// m.Now is before it).
	//
	// Note that m.Now is part of inputs and must not be mutated. We propagate
	// the corrected time via the call stack.
	now := m.Now
	switch delay := now.Sub(m.State.LastTick.When); {
	case delay < -50*time.Millisecond:
		return fmt.Errorf("tick happened %s before it was expected", -delay)
	case delay < 0:
		now = m.State.LastTick.When
	}

	// The scheduled time has come!
	m.State.Generation = m.nextGen()
	m.Actions = append(m.Actions, StartInvocationAction{
		Generation: m.State.Generation,
	})
	m.State.LastTick = TickLaterAction{}

	// Start waiting for a new tick right away if on an absolute schedule or just
	// keep the tick state clear for relative schedules: new tick will be set when
	// RewindIfNecessary() is manually called by whoever handles the cron.
	if m.Schedule.IsAbsolute() {
		m.rewindIfNecessary(now)
	}

	return nil
}

// scheduleTick emits TickLaterAction action according to the schedule, current
// time, and last time RewindIfNecessary was called.
//
// Does nothing if such tick has already been scheduled.
func (m *Machine) scheduleTick(now time.Time) {
	nextTickTime := m.Schedule.Next(now, m.State.LastRewind)
	if nextTickTime != m.State.LastTick.When {
		m.State.Generation = m.nextGen()
		m.State.LastTick = TickLaterAction{
			When:      nextTickTime,
			TickNonce: m.Nonce(),
		}
		if nextTickTime != schedule.DistantFuture {
			m.Actions = append(m.Actions, m.State.LastTick)
		}
	}
}

// nextGen returns the next generation number.
//
// It does NOT update Generation in-place, just produces the next number.
func (m *Machine) nextGen() int64 {
	return m.State.Generation + 1
}

// rewindIfNecessary implements RewindIfNecessary, accepting corrected 'now'.
//
// This is important for OnTimerTick. Note that m.Now is part of inputs and must
// not be mutated.
func (m *Machine) rewindIfNecessary(now time.Time) {
	if m.State.Enabled && m.State.LastTick.When.IsZero() {
		m.State.LastRewind = now
		m.State.Generation = m.nextGen()
		m.scheduleTick(now)
	}
}
