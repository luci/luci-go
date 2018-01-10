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

	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/errors"
)

// State represents the state of a resource in the Machine Database.
type State int

const (
	// Free indicates a resource is not allocated.
	Free State = iota + 1
	// Prerelease indicates a resource is allocated for future use.
	Prerelease
	// Serving indicates a resource is allocated and currently used in production.
	Serving
	// Test indicates a resource is allocated and currently used for testing.
	Test
	// Repair indicates a resource is undergoing repairs.
	Repair
	// Decommissioned indicates a resource is allocated but unused.
	Decommissioned
)

// String returns a human-readable string representation of this state.
func (s State) String() string {
	switch s {
	case Free:
		return "free"
	case Prerelease:
		return "prerelease"
	case Serving:
		return "serving"
	case Test:
		return "test"
	case Repair:
		return "repair"
	case Decommissioned:
		return "decommissioned"
	}
	return ""
}

// GetState returns a State given its human-readable representation. Supports prefix matching.
func GetState(s string) (State, error) {
	switch {
	case s == "":
		// Don't allow the empty string to match.
	case strings.HasPrefix("free", s):
		return Free, nil
	case strings.HasPrefix("prerelease", s):
		return Prerelease, nil
	case strings.HasPrefix("serving", s):
		return Serving, nil
	case strings.HasPrefix("test", s):
		return Test, nil
	case strings.HasPrefix("repair", s):
		return Repair, nil
	case strings.HasPrefix("decommissioned", s):
		return Decommissioned, nil
	}
	return -1, errors.Reason("string %q did not match any known state", s).Err()
}

// ValidateState validates the given state, allowing empty/unspecified state.
func ValidateState(c *validation.Context, s string) {
	if len(s) > 0 {
		if _, err := GetState(s); err != nil {
			c.Errorf("invalid state %q", s)
		}
	}
}
