// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dm

import (
	"fmt"
)

// validAttemptStateEvolution defines all valid {From -> []To} state
// transitions. The identity transition (X -> X) is implied, as long as X has an
// entry in this mapping.
var validAttemptStateEvolution = map[Attempt_State][]Attempt_State{
	Attempt_ADDING_DEPS:              {Attempt_BLOCKED, Attempt_NEEDS_EXECUTION},
	Attempt_BLOCKED:                  {Attempt_AWAITING_EXECUTION_STATE, Attempt_NEEDS_EXECUTION},
	Attempt_AWAITING_EXECUTION_STATE: {Attempt_NEEDS_EXECUTION},
	Attempt_EXECUTING:                {Attempt_ADDING_DEPS, Attempt_FINISHED},
	Attempt_FINISHED:                 {},
	Attempt_NEEDS_EXECUTION:          {Attempt_EXECUTING},
}

// Evolve attempts to evolve the state of this Attempt. If the state evolution
// is not allowed (e.g. invalid state transition), this returns an error.
func (s *Attempt_State) Evolve(newState Attempt_State) error {
	nextStates := validAttemptStateEvolution[*s]
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
func (s *Attempt_State) MustEvolve(newState Attempt_State) {
	err := s.Evolve(newState)
	if err != nil {
		panic(err)
	}
}

// Terminal returns true iff there are no valid evolutions from the current
// state.
func (s Attempt_State) Terminal() bool {
	return len(validAttemptStateEvolution[s]) == 0
}
