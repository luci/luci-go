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
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Name returns a string which can be used as the human-readable representation expected by GetState.
func (s State) Name() string {
	switch s {
	case State_STATE_UNSPECIFIED:
		return ""
	case State_FREE:
		return "free"
	case State_PRERELEASE:
		return "prerelease"
	case State_SERVING:
		return "serving"
	case State_TEST:
		return "test"
	case State_REPAIR:
		return "repair"
	case State_DECOMMISSIONED:
		return "decommissioned"
	default:
		return "invalid state"
	}
}

// GetState returns a State given its name. Supports prefix matching.
func GetState(s string) (State, error) {
	lower := strings.ToLower(s)
	switch {
	case lower == "":
		return State_STATE_UNSPECIFIED, nil
	case strings.HasPrefix("free", lower):
		return State_FREE, nil
	case strings.HasPrefix("prerelease", lower):
		return State_PRERELEASE, nil
	case strings.HasPrefix("serving", lower):
		return State_SERVING, nil
	case strings.HasPrefix("test", lower):
		return State_TEST, nil
	case strings.HasPrefix("repair", lower):
		return State_REPAIR, nil
	case strings.HasPrefix("decommissioned", lower):
		return State_DECOMMISSIONED, nil
	default:
		return -1, errors.Reason("string %q did not match any known state", s).Err()
	}
}

// ValidStates returns a slice of valid states.
func ValidStates() []State {
	return []State{State_FREE, State_PRERELEASE, State_SERVING, State_TEST, State_REPAIR, State_DECOMMISSIONED}
}
