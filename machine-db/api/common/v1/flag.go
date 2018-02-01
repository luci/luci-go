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

package common

import (
	"flag"

	"go.chromium.org/luci/common/errors"
)

// stateFlag is a State which implements the flag.Value interface.
type stateFlag State

// Set stores a new State in this flag, overriding any existing State.
func (f *stateFlag) Set(s string) error {
	i, err := GetState(s)
	if err != nil {
		return errors.Annotate(err, "value must be a valid State").Err()
	}
	*f = stateFlag(i)
	return nil
}

// String returns a string which can be used to Set another stateFlag to store the same State.
func (f *stateFlag) String() string {
	switch s := State(*f); {
	case s == State_STATE_UNSPECIFIED:
		return ""
	case s == State_FREE:
		return "free"
	case s == State_PRERELEASE:
		return "prerelease"
	case s == State_SERVING:
		return "serving"
	case s == State_TEST:
		return "test"
	case s == State_REPAIR:
		return "repair"
	case s == State_DECOMMISSIONED:
		return "decommissioned"
	default:
		return "invalid state"
	}
}

// StateFlag returns a flag.Value which reads flag values into the given *State.
func StateFlag(s *State) flag.Value {
	return (*stateFlag)(s)
}
