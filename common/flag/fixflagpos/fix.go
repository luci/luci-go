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

package fixflagpos

import (
	"strings"
)

// Fix rearranges an `args []string` command line to enable positional arguments
// to be allowed before flag arguments, which makes some invocations look more
// natural.
//
// Converts [pos1 pos2 -flag value pos3] into [-flag value pos3 pos1 pos2].
func Fix(args []string) []string {
	// Find the first flag argument or '--'.
	flagIdx := -1
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flagIdx = i
			break
		}
	}

	if flagIdx == -1 {
		return args // no arguments at all or only positional arguments
	}

	// Move positional arguments after the flags.
	buf := make([]string, 0, len(args))
	return append(append(buf, args[flagIdx:]...), args[:flagIdx]...)
}

// FixSubcommands is like Fix, except the first positional argument is
// understood as a subcommand name and it is not moved.
//
// Converts [pos1 pos2 -flag value pos3] into [pos1 -flag value pos3 pos2].
//
// Taking cipd as an example, compare:
//   * Default: cipd set-ref -ref=abc -version=def package/name
//   * Improved: cipd set-ref package/name -ref=abc -version=def
//
// Much better.
func FixSubcommands(args []string) []string {
	// Nothing do it if the first argument is a flag.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return args
	}
	buf := make([]string, 0, len(args))
	return append(append(buf, args[0]), Fix(args[1:])...)
}
