// Copyright 2021 The LUCI Authors.
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

package gcloud

import (
	"errors"
	"fmt"
)

// ValidateProjectID returns an error if the supplied string is not
// a valid Google Cloud project ID.
//
// A valid project ID
// - must be 6 to 30 lowercase ASCII letters, digits, or hyphens,
// - must start with a letter, and
// - must not have a trailing hyphen.
//
// See:
// https://cloud.google.com/resource-manager/reference/rest/v3/projects#resource:-project
func ValidateProjectID(p string) error {
	if len(p) < 6 || len(p) > 30 {
		return errors.New("must contain 6 to 30 ASCII letters, digits, or hyphens")
	}
	if p[0] < 'a' || p[0] > 'z' {
		return errors.New("must start with a lowercase ASCII letter")
	}
	if p[len(p)-1] == '-' {
		return errors.New("must not have a trailing hyphen")
	}
	for i := 1; i < len(p)-1; i++ {
		switch {
		case p[i] >= 'a' && p[i] <= 'z', p[i] >= '0' && p[i] <= '9', p[i] == '-':
		default:
			return fmt.Errorf("invalid letter at %d (%c)", i, p[i])
		}
	}
	return nil
}
