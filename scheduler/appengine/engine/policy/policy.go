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

// Package policy contains implementation of triggering policy functions.
package policy

import (
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// Environment is used by the triggering policy for getting transient
// information about the environment and for logging.
//
// TODO(vadimsh): This is intentionally mostly empty for now. Will be extended
// on as-needed basis.
type Environment interface {
	// DebugLog appends a line to the triage text log.
	DebugLog(format string, args ...interface{})
}

// In contains parameters for a triggering policy function.
type In struct {
	// Now is the time when the policy function was called.
	Now time.Time
	// ActiveInvocations is a set of currently running invocations of the job.
	ActiveInvocations []int64
	// Triggers is a list of pending triggers sorted by time, more recent last.
	Triggers []*internal.Trigger
}

// Out contains the decision of a triggering policy function.
type Out struct {
	// Requests is a list of requests to start new invocations (if any).
	//
	// Each request contains parameters that will be passed to a new invocation by
	// the engine. The policy is responsible for filling them in.
	//
	// Triggers specified in the each request will be removed from the set of
	// pending triggers (they are consumed).
	Requests []task.Request
}

// Func is the concrete implementation of a triggering policy.
//
// It looks at the current state of the job and its pending triggers list and
// (optionally) emits a bunch of requests to start new invocations.
//
// It is a pure function without any side effects (except, perhaps, logging into
// the given environment).
type Func func(Environment, In) Out

// New is a factory that takes TriggeringPolicy proto and returns a concrete
// function that implements this policy.
//
// The returned function will be used only during one triage round and then
// discarded.
//
// Returns an error if the TriggeringPolicy message can't be understood (for
// example, it references an undefined policy kind).
func New(p messages.TriggeringPolicy) (Func, error) {
	// Fill in defaults.
	if p.Kind == 0 {
		p.Kind = messages.TriggeringPolicy_GREEDY_BATCHING
	}
	if p.MaxConcurrentInvocations == 0 {
		p.MaxConcurrentInvocations = 1
	}

	// There will be more kinds here, so this should technically be a switch, even
	// though currently it is not a very interesting switch.
	switch p.Kind {
	case messages.TriggeringPolicy_GREEDY_BATCHING:
		return GreedyBatchingPolicy(int(p.MaxConcurrentInvocations))
	default:
		return nil, errors.Reason("unrecognized triggering policy kind %d", p.Kind).Err()
	}
}
