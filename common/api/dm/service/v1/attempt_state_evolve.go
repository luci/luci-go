// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"fmt"
)

// validAttemptStateEvolution defines all valid {From -> []To} state
// transitions. The identity transition (X -> X) is implied, as long as X has an
// entry in this mapping.
var validAttemptStateEvolution = map[Attempt_State][]Attempt_State{
	Attempt_AddingDeps:     {Attempt_Blocked, Attempt_NeedsExecution},
	Attempt_Blocked:        {Attempt_NeedsExecution},
	Attempt_Executing:      {Attempt_AddingDeps, Attempt_Finished},
	Attempt_Finished:       {},
	Attempt_NeedsExecution: {Attempt_Executing},
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
