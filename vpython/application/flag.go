// Copyright 2017 The LUCI Authors.
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

package application

import (
	"flag"
	"strings"
)

// boolFlag is an interface implemented by boolean flag Value instances. We use
// this to determine if a flag is a boolean flag.
//
// This is copied from:
// https://github.com/golang/go/blob/8d63408f4688ff577c25f07a1728fe131d0cae2a/src/flag/flag.go#L101
type boolFlag interface {
	flag.Value
	IsBoolFlag() bool
}

func extractFlagsForSet(args []string, fs *flag.FlagSet) (fsArgs, remainder []string) {
	// Fast paths.
	switch {
	case len(args) == 0:
		return
	case len(args[0]) == 0 || args[0][0] != '-':
		// If our first argument isn't a flag, then everything belongs to
		// "remainder", no processing necessary.
		remainder = args
		return
	}

	// Scan "args" for a "--" divider. We only process candidates to the left of
	// the divider.
	candidates := args
	for i, arg := range args {
		if arg == "--" {
			candidates = args[:i]
			break
		}
	}
	if len(candidates) == 0 {
		remainder = args
		return
	}

	// Make a map of all registered flags in "fs". The value will be "true" if
	// the flag is a boolean flag, and "false" if it is not.
	flags := make(map[string]bool)
	fs.VisitAll(func(f *flag.Flag) {
		bf, ok := f.Value.(boolFlag)
		flags[f.Name] = ok && bf.IsBoolFlag()
	})

	processOne := func(args []string) int {
		if len(args) == 0 {
			return 0
		}
		arg := args[0]

		numMinuses := 0
		if len(arg) > 0 && arg[0] == '-' {
			if len(arg) > 1 && arg[1] == '-' {
				numMinuses = 2
			} else {
				numMinuses = 1
			}
		}
		arg = arg[numMinuses:]

		if numMinuses == 0 || len(arg) == 0 {
			// Not a flag, so we're done.
			return 0
		}

		single := false
		eqIdx := strings.IndexRune(arg, '=')
		if eqIdx >= 0 {
			single = true
			arg = arg[:eqIdx]
		}

		flagIsBool, ok := flags[arg]
		if !ok {
			// Unknown flag.
			return 0
		}
		if flagIsBool || single {
			return 1
		}
		return 2
	}

	for i := 0; i < len(candidates); {
		consume := processOne(candidates[i:])
		if consume == 0 {
			fsArgs, remainder = args[:i], args[i:]
			return
		}
		i += consume
	}

	// Got to the end with no non-"fs" flags, so everything goes to fsArgs.
	fsArgs, remainder = candidates, args[len(candidates):]
	return
}
