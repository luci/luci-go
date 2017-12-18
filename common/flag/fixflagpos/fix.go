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

	"github.com/maruel/subcommands"
)

// IsCommandFn will be invoked with successive groups of tokens in order to
// determine what portion of the command line is a 'subcommand'. E.g. for the
// command line:
//
//   prog show cool subcommand option -flag 1
//
// This would be invoked with:
//   * ['show']
//   * ['show', 'cool']
//   * ['show', 'cool', 'subcommand']
//   * ['show', 'cool', 'subcommand', 'option']
//
// And the function would return `true` for the first three and `false` for the
// fourth. This way FixSubcommands could know that 'option' is a positional
// argument and not a subcommand.
type IsCommandFn func(toks []string) bool

// MaruelSubcommandsFn returns an IsCommandFn which is compatible with
// "maruel/subcommands".Application objects.
func MaruelSubcommandsFn(a subcommands.Application) IsCommandFn {
	return func(toks []string) bool {
		if len(toks) == 1 {
			for _, c := range a.GetCommands() {
				if toks[0] == c.Name() {
					return true
				}
			}
		}
		return false
	}
}

func splitCmdLine(args []string, isCommand IsCommandFn) (subcmd []string, flags []string, pos []string) {
	// No subcomand, just flags.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return nil, args, nil
	}

	if isCommand != nil {
		for len(args) > 0 {
			maybeSub := append(subcmd, args[0])
			if !isCommand(maybeSub) {
				break
			}
			args = args[1:]
			subcmd = maybeSub
		}
	}

	// Pick subcommand, than collect all positional args up to a first flag.
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		pos = append(pos, args[0])
		args = args[1:]
	}

	flags = args

	return
}

// FixSubcommands rearanges an `args []string` command line to enable flag
// arguments and positional arguments to be mixed, which makes some invocations
// look more natural.
//
// Taking cipd as an example, compare:
//   * Default: cipd set-ref -ref=abc -version=def package/name
//   * Improved: cipd set-ref package/name -ref=abc -version=def
//
// Much better.
//
// Pass a function isCommand which should return true if the tokens in `toks`
// indicate a subcommand. As long as this function returns true, FixSubcommands
// will keep these tokens at the beginning of the returned slice. As soon as it
// returns false, the remaining tokens will be sorted into flags and positional
// arguments, and the positional arguments will be moved to the end of the
// slice. Passing `nil` means that no tokens will be treated as subcommands (do
// this if you're not using a subcommand-parsing library).
func FixSubcommands(args []string, isCommand IsCommandFn) []string {
	cmd, flags, positional := splitCmdLine(args, isCommand)
	length := len(cmd) + len(flags) + len(positional)
	if length == 0 {
		return nil
	}
	buf := make([]string, 0, length)
	return append(append(append(buf, cmd...), flags...), positional...)
}

// Fix is a shorthand for `FixSubcommands(args, nil)`.
func Fix(args []string) []string {
	return FixSubcommands(args, nil)
}
