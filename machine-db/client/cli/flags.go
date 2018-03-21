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
	"sort"
	"strings"

	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/common/v1"
)

// CommonFlags contains common flags for all commands.
type CommonFlags struct {
	tsv bool
}

// Register registers common flags with the given flag.FlagSet.
func (f *CommonFlags) Register(flags *flag.FlagSet) {
	flags.BoolVar(&f.tsv, "tsv", false, "Whether to emit data in tsv instead of human-readable format.")
}

// getUpdateMask returns a *field_mask.FieldMask containing paths based on which flags have been set.
func getUpdateMask(set *flag.FlagSet, paths map[string]string) *field_mask.FieldMask {
	m := &field_mask.FieldMask{}
	set.Visit(func(f *flag.Flag) {
		if path, ok := paths[f.Name]; ok {
			m.Paths = append(m.Paths, path)
		}
	})
	sort.Strings(m.Paths)
	return m
}

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

// stateSliceFlag is a []common.State which implements the flag.Value interface.
type stateSliceFlag []common.State

// Set appends a new state to this flag, preserving any existing states.
func (f *stateSliceFlag) Set(s string) error {
	i, err := common.GetState(s)
	if err != nil {
		return errors.Reason("value must be a valid state returned by get-states").Err()
	}
	*f = append(*f, i)
	return nil
}

// String returns a comma-separated string representation of flag.
func (f *stateSliceFlag) String() string {
	s := make([]string, len(*f))
	for i, j := range *f {
		s[i] = j.Name()
	}
	return strings.Join(s, ", ")
}

// StateSliceFlag returns a flag.Value which appends flag values to the given *[]common.State.
func StateSliceFlag(s *[]common.State) flag.Value {
	return (*stateSliceFlag)(s)
}
