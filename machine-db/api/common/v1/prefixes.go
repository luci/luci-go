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

// GetState returns a State given its human-readable representation. Supports prefix matching.
func GetState(s string) (State, error) {
	s = strings.ToLower(s)
	switch {
	case s == "":
		// Don't allow the empty string to match.
	case strings.HasPrefix("unspecified", s):
		return State_UNSPECIFIED, nil
	case strings.HasPrefix("free", s):
		return State_FREE, nil
	case strings.HasPrefix("prerelease", s):
		return State_PRERELEASE, nil
	case strings.HasPrefix("serving", s):
		return State_SERVING, nil
	case strings.HasPrefix("test", s):
		return State_TEST, nil
	case strings.HasPrefix("repair", s):
		return State_REPAIR, nil
	case strings.HasPrefix("decommissioned", s):
		return State_DECOMMISSIONED, nil
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
