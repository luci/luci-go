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

package dm

import (
	"fmt"
)

// validExecutionStateEvolution defines all valid {From -> []To} state
// transitions. The identity transition (X -> X) is implied, as long as X has an
// entry in this mapping.
var validExecutionStateEvolution = map[Execution_State][]Execution_State{
	Execution_SCHEDULING: {
		Execution_RUNNING,           // ActivateExecution
		Execution_ABNORMAL_FINISHED, // cancel/timeout/err/etc.
	},
	Execution_RUNNING: {
		Execution_STOPPING,          // FinishAttempt/EnsureGraphData
		Execution_ABNORMAL_FINISHED, // cancel/timeout/err/etc.
	},
	Execution_STOPPING: {
		Execution_FINISHED,          // got persistent state from distributor
		Execution_ABNORMAL_FINISHED, // cancel/timeout/err/etc.
	},

	Execution_FINISHED:          {},
	Execution_ABNORMAL_FINISHED: {},
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

// Terminal returns true iff there are no valid evolutions from the current
// state.
func (s Execution_State) Terminal() bool {
	return len(validExecutionStateEvolution[s]) == 0
}
