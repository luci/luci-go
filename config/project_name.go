// Copyright 2016 The LUCI Authors.
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

package config

import (
	"errors"
	"fmt"
)

// ValidateProjectName returns an error if the supplied string is not a valid
// project name.
//
// A valid project name may only include:
//   - Lowercase letters [a-z]
//   - Numbers [0-9]
//   - Hyphen (-)
//   - Underscore (_)
//
// It also must begin with a letter.
//
// See:
// https://github.com/luci/luci-py/blob/8e594074929871a9761d27e814541bc0d7d84744/appengine/components/components/config/common.py#L41
func ValidateProjectName(p string) error {
	if len(p) == 0 {
		return errors.New("cannot have empty name")
	}

	for idx, r := range p {
		switch {
		case r >= 'a' && r <= 'z':

		case (r >= '0' && r <= '9'), r == '-', r == '_':
			if idx == 0 {
				return errors.New("must begin with a letter")
			}

		default:
			return fmt.Errorf("invalid character at %d (%c)", idx, r)
		}
	}

	return nil
}
