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

package nestedflagset

import (
	"flag"
	"fmt"
	"strings"
)

// FlagSet is a flag.Flag implementation that parses into a nested flag.FlagSet.
//
// The FlagSet's flag value is parsed broken into a series of sub-options, which
// are then loaded into the nested flag set (FlagSet.F).
type FlagSet struct {
	F flag.FlagSet // The underlying FlagSet, populated on Parse.
}

var _ flag.Value = (*FlagSet)(nil)

// Parse parses a one-line option string into the underlying flag set.
func (fs *FlagSet) Parse(line string) error {
	var args []string
	for _, token := range lexer(line, ',').split() {
		name, value := token.split()
		args = append(args, fmt.Sprintf("-%s", name))
		if value != "" {
			args = append(args, value)
		}
	}

	return fs.F.Parse(args)
}

// Usage constructs a one-line usage string for all of the options defined in
// Flags.
func (fs *FlagSet) Usage() string {
	var flags []string

	// If there is no "help" flag defined, we will use it to display help/usage
	// (default FlagSet).
	if fs.F.Lookup("help") == nil {
		flags = append(flags, "help")
	}

	fs.F.VisitAll(func(f *flag.Flag) {
		comma := ""
		if len(flags) > 0 {
			comma = ","
		}
		flags = append(flags, fmt.Sprintf("[%s%s]", comma, f.Name))
	})
	return strings.Join(flags, "")
}

// Set implements flags.Value.
func (fs *FlagSet) Set(value string) error {
	return fs.Parse(value)
}

// String implements flags.Value.
func (fs *FlagSet) String() string {
	return fs.Usage()
}
