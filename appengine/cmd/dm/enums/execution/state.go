// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate stringer -type=State

package execution

import (
	"fmt"
)

// State is the state that a single Execution can be in.
type State int8

// State values
const (
	UnknownState State = iota

	Scheduled
	Running

	Rejected
	Finished
	Crashed
)

// validStateEvolution defines all valid {From -> []To} state transitions. The
// identity transition (X -> X) is implied, as long as X has an entry in this
// mapping.
var validStateEvolution = map[State][]State{
	Scheduled: {Running, Rejected, Crashed, Finished},
	Running:   {Crashed, Finished, Rejected},

	Crashed:  {},
	Finished: {},
	Rejected: {},
}

// Evolve attempts to evolve the state of this Attempt. If the state
// evolution is not allowed (e.g. invalid state transition), this returns an
// error.
func (s *State) Evolve(newState State) error {
	nextStates := validStateEvolution[*s]
	if nextStates == nil {
		return fmt.Errorf("invalid state transition: no transitions defined for %s", *s)
	}

	if newState == *s {
		return nil
	}

	for _, val := range nextStates {
		if newState == val {
			*s = newState
			return nil
		}
	}

	return fmt.Errorf("invalid state transition %v -> %v", *s, newState)
}

// MustEvolve is a panic'ing version of Evolve.
func (s *State) MustEvolve(newState State) {
	err := s.Evolve(newState)
	if err != nil {
		panic(err)
	}
}
