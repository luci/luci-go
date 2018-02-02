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

package cli

import (
	"flag"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/common/v1"
)

// stateFlag is a common.State which implements the flag.Value interface.
type stateFlag common.State

// Set stores a new state in this flag, overriding any existing state.
func (f *stateFlag) Set(s string) error {
	i, err := common.GetState(s)
	if err != nil {
		return errors.Reason("value must be a valid state returned by get-states").Err()
	}
	*f = stateFlag(i)
	return nil
}

// String returns a string which can be used to Set another stateFlag to store the same state.
func (f *stateFlag) String() string {
	return common.State(*f).Name()
}

// StateFlag returns a flag.Value which reads flag values into the given *common.State.
func StateFlag(s *common.State) flag.Value {
	return (*stateFlag)(s)
}
