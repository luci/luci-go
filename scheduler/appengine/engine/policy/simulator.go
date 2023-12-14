// Copyright 2018 The LUCI Authors.
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

package policy

import (
	"container/heap"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// Simulator is used to test policies.
//
// It simulates the scheduler engine logic and passage of time. It takes a
// stream of triggers as input, passes them through the policy under the test,
// and collects the resulting invocation requests.
type Simulator struct {
	// Policy is the policy function under test.
	//
	// Must be set by the caller.
	Policy Func

	// OnRequest is called whenever a new invocation request is emitted by the
	// policy.
	//
	// It decides for how long the invocation will run.
	//
	// Must be set by the caller.
	OnRequest func(s *Simulator, r task.Request) time.Duration

	// OnDebugLog is called whenever the triggering policy logs something.
	//
	// May be set by the caller to collect the policy logs.
	OnDebugLog func(format string, args ...any)

	// Epoch is the timestamp of when the simulation started.
	//
	// Used to calculate SimulatedInvocation.Created. It is fine to leave it
	// default if you aren't looking at absolute times (which will be weird with
	// zero epoch time).
	Epoch time.Time

	// Now is the current time inside the simulation.
	//
	// It is advanced on various events (like new triggers or finishing
	// invocations). Use AdvanceTime to move it manually.
	Now time.Time

	// PendingTriggers is a set of currently pending triggers, sorted by time
	// (most recent last).
	//
	// Do not modify this list directly, use AddTrigger instead.
	PendingTriggers []*internal.Trigger

	// Invocations is a log of all produced invocations.
	//
	// They are ordered by the creation time. Contains invocations that are still
	// running (based on Now). Use Last() as a shortcut to get the last item of
	// this list.
	Invocations []*SimulatedInvocation

	// DiscardedTriggers is a log of all triggers that were discarded, sorted by time
	// (most recent last).
	DiscardedTriggers []*internal.Trigger

	// Internals.

	// events is a priority queue (heap) of future events.
	events events
	// seenTriggers is a set of IDs of all triggers ever seen, for deduplication.
	seenTriggers stringset.Set
	// nextInvID is used by handleRequest.
	nextInvID int64
	// invIDs is a set of running invocations.
	invIDs map[int64]*SimulatedInvocation
}

// SimulatedInvocation contains details of an invocation.
type SimulatedInvocation struct {
	// Request is the original invocation request as emitted by the policy.
	Request task.Request
	// Created is when the invocation was created, relative to the epoch.
	Created time.Duration
	// Duration of the invocation, as returned by OnRequest.
	Duration time.Duration
	// Running is true if the invocation is still running.
	Running bool
}

// SimulatedEnvironment implements Environment interface for use by Simulator.
type SimulatedEnvironment struct {
	OnDebugLog func(format string, args ...any)
}

// DebugLog is part of Environment interface.
func (s *SimulatedEnvironment) DebugLog(format string, args ...any) {
	if s.OnDebugLog != nil {
		s.OnDebugLog(format, args...)
	}
}

////////////////////////////////////////////////////////////////////////////////
// Triggering and invocations.

// Last returns the last invocation in Invocations list or nil if its empty.
func (s *Simulator) Last() *SimulatedInvocation {
	if len(s.Invocations) == 0 {
		return nil
	}
	return s.Invocations[len(s.Invocations)-1]
}

// AddTrigger submits a trigger (one or many) to the pending trigger set.
//
// This causes the execution of the policy function to decide what to do with
// the new triggers.
//
// 'delay' is time interval from the previously submitted trigger. It is used to
// advance time. The current simulation time will be used to populate trigger's
// Created field.
func (s *Simulator) AddTrigger(delay time.Duration, t ...internal.Trigger) {
	s.AdvanceTime(delay)

	if s.seenTriggers == nil {
		s.seenTriggers = stringset.New(0)
	}

	ts := timestamppb.New(s.Now)
	for _, tr := range t {
		tr := proto.Clone(&tr).(*internal.Trigger)
		tr.Created = ts
		if s.seenTriggers.Add(tr.Id) {
			s.PendingTriggers = append(s.PendingTriggers, tr)
		}
	}

	s.triage()
}

// triage executes the triggering policy function.
func (s *Simulator) triage() {
	// Collect the unordered list of currently running invocations.
	invs := make([]int64, 0, len(s.invIDs))
	for id := range s.invIDs {
		invs = append(invs, id)
	}

	// Clone pending triggers list since we don't want the policy to mutate them.
	triggers := make([]*internal.Trigger, len(s.PendingTriggers))
	for i, t := range s.PendingTriggers {
		triggers[i] = proto.Clone(t).(*internal.Trigger)
	}

	// Execute the policy function, collecting its log.
	out := s.Policy(&SimulatedEnvironment{s.OnDebugLog}, In{
		Now:               s.Now,
		ActiveInvocations: invs,
		Triggers:          triggers,
	})

	// Instantiate all new invocations and collect a set of consumed triggers.
	consumed := stringset.New(0)
	for _, r := range out.Requests {
		s.handleRequest(r)
		for _, t := range r.IncomingTriggers {
			consumed.Add(t.Id)
		}
	}

	// Collect a set of discarded triggers.
	discarded := stringset.New(0)
	for _, t := range out.Discard {
		discarded.Add(t.Id)
	}

	// Pop all consumed or discarded triggers from PendingTriggers list (keeping it sorted).
	if consumed.Len() != 0 || discarded.Len() != 0 {
		filtered := make([]*internal.Trigger, 0, len(s.PendingTriggers))
		for _, t := range s.PendingTriggers {
			if !consumed.Has(t.Id) && !discarded.Has(t.Id) {
				filtered = append(filtered, t)
			}
			if discarded.Has(t.Id) {
				s.DiscardedTriggers = append(s.DiscardedTriggers, t)
			}
		}
		s.PendingTriggers = filtered
	}
}

// handleRequest is called for each invocation request created by the policy.
//
// It adds new SimulatedInvocation to Invocations list.
func (s *Simulator) handleRequest(r task.Request) {
	dur := s.OnRequest(s, r)
	if dur <= 0 {
		panic("the invocation duration should be positive")
	}

	inv := &SimulatedInvocation{
		Request:  r,
		Created:  s.Now.Sub(s.Epoch),
		Duration: dur,
		Running:  true,
	}
	s.Invocations = append(s.Invocations, inv)

	s.nextInvID++
	id := s.nextInvID
	if s.invIDs == nil {
		s.invIDs = map[int64]*SimulatedInvocation{}
	}
	s.invIDs[id] = inv

	s.scheduleEvent(event{
		eta: s.Now.Add(inv.Duration),
		cb: func() {
			// On invocation completion, kick it from the active invocations set and
			// rerun the triggering policy function to decide what to do next.
			inv.Running = false
			delete(s.invIDs, id)
			s.triage()
		},
	})
}

////////////////////////////////////////////////////////////////////////////////
// Event reactor.

// event sits in a timeline and its callback is executed at moment 'eta'.
type event struct {
	eta time.Time
	cb  func()
}

// events implements heap.Interface, smallest eta is on top of the heap.
type events []event

func (e events) Len() int           { return len(e) }
func (e events) Less(i, j int) bool { return e[i].eta.Before(e[j].eta) }
func (e events) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }
func (e *events) Push(x any)        { *e = append(*e, x.(event)) }
func (e *events) Pop() any {
	old := *e
	n := len(old)
	x := old[n-1]
	*e = old[0 : n-1]
	return x
}

// AdvanceTime moves the simulated time, executing all events that happen.
func (s *Simulator) AdvanceTime(d time.Duration) {
	switch {
	case d == 0:
		return
	case d < 0:
		panic("time must move forward only")
	}

	// First tick ever? Reset Now to Epoch, since Epoch is our beginning of times.
	if s.Now.IsZero() {
		s.Now = s.Epoch
	}

	deadline := s.Now.Add(d)
	for {
		// Nothing is happening at all or events happen later than we wish to go?
		if ev := s.peekEvent(); ev == nil || ev.eta.After(deadline) {
			s.Now = deadline
			return
		}

		// Advance the time to the point when the event is happening and execute the
		// event's callback. It may result in most stuff added to the timeline which
		// we will discover on the next iteration of the loop.
		ev := s.popEvent()
		s.Now = ev.eta
		ev.cb()
	}
}

// scheduleEvent adds an event to the event queue.
//
// Panics if event's ETA is not in the future.
func (s *Simulator) scheduleEvent(e event) {
	if !e.eta.After(s.Now) {
		panic("event's ETA should be in the future")
	}
	heap.Push(&s.events, e)
}

// peekEvent peeks at the event that happens next.
//
// Returns nil if there are no pending events.
func (s *Simulator) peekEvent() *event {
	if len(s.events) == 0 {
		return nil
	}
	return &s.events[0]
}

// popEvent removes the event that happens next.
//
// Panics if there's no pending events.
func (s *Simulator) popEvent() event {
	if len(s.events) == 0 {
		panic("no events to pop")
	}
	return heap.Pop(&s.events).(event)
}
