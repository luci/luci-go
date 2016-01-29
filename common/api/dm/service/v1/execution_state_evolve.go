// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"fmt"
)

// validExecutionStateEvolution defines all valid {From -> []To} state
// transitions. The identity transition (X -> X) is implied, as long as X has an
// entry in this mapping.
var validExecutionStateEvolution = map[Execution_State][]Execution_State{
	Execution_Scheduled: {Execution_Running, Execution_Rejected, Execution_Crashed, Execution_Finished},
	Execution_Running:   {Execution_Crashed, Execution_Finished, Execution_Rejected},

	Execution_Crashed:  {},
	Execution_Finished: {},
	Execution_Rejected: {},
}

// Evolve attempts to evolve the state of this Attempt. If the state
// evolution is not allowed (e.g. invalid state transition), this returns an
// error.
func (s *Execution_State) Evolve(newState Execution_State) error {
	nextStates := validExecutionStateEvolution[*s]
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
func (s *Execution_State) MustEvolve(newState Execution_State) {
	err := s.Evolve(newState)
	if err != nil {
		panic(err)
	}
}
