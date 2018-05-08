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

package gerrit

import (
	"go.chromium.org/luci/common/errors"
)

// Validate returns an error if r is invalid.
func (r *CheckAccessRequest) Validate() error {
	switch {
	case r.Project == "":
		return errors.New("project is required")
	case r.Permission == "":
		return errors.New("permission is required")
	case r.Account == "":
		return errors.New("account is required")
	default:
		return nil
	}
}

// Validate returns an error if r is invalid.
func (r *GetChangeRequest) Validate() error {
	switch {
	case r.Number <= 0:
		return errors.New("number must be positive")
	default:
		return nil
	}
}
