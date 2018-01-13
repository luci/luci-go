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

package python

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"go.chromium.org/luci/common/errors"
)

// CommandLineFlag is a command-line flag and its associated argument, if one
// is provided.
type CommandLineFlag struct {
	Flag string
	Arg  string
}

func (f *CommandLineFlag) String() string {
	if f.Arg != "" {
		return fmt.Sprintf("-%s%s", f.Flag, f.Arg)
	}
	return fmt.Sprintf("-%s", f.Flag)
}

// Target describes a Python invocation target.
//
// Targets are identified by parsing a Python command-line using
// ParseCommandLine.
//
// A Target is identified through type assertion, and will be one of:
//
//	- NoTarget
//	- ScriptTarget
//	- CommandTarget
//	- ModuleTarget
type Target interface {
	// buildArgsForTarget returns the arguments to pass to the interpreter to
	// invoke this target.
	buildArgsForTarget() []string
}

// NoTarget is a Target implementation indicating no Python target (i.e.,
// interactive).
type NoTarget struct{}

func (NoTarget) buildArgsForTarget() []string { return nil }

// ScriptTarget is a Python executable script target.
type ScriptTarget struct {
	// Path is the path to the script that is being invoked.
	//
	// This may be "-", indicating that the script is being read from STDIN.
	Path string
}

func (t ScriptTarget) buildArgsForTarget() []string { return []string{t.Path} }

// CommandTarget is a Target implementation for a command-line string
// (-c ...).
type CommandTarget struct {
	// Command is the command contents.
	Command string
}

func (t CommandTarget) buildArgsForTarget() []string { return []string{"-c", t.Command} }

// ModuleTarget is a Target implementating indicating a Python module (-m ...).
type ModuleTarget struct {
	// Module is the name of the target module.
	Module string
}

func (t ModuleTarget) buildArgsForTarget() []string { return []string{"-m", t.Module} }

// CommandLine is a parsed Python command-line.
//
// CommandLine can be parsed from arguments via ParseCommandLine.
type CommandLine struct {
	// Target is the Python target type.
	Target Target

	// Flags are flags to the Python interpreter.
	Flags []CommandLineFlag
	// Args are arguments passed to the Python script.
	Args []string
}

// BuildArgs returns an array of Python interpreter arguments for cl.
func (cl *CommandLine) BuildArgs() []string {
	targetArgs := cl.Target.buildArgsForTarget()
	args := make([]string, 0, len(cl.Flags)+len(targetArgs)+len(cl.Args))
	for _, flag := range cl.Flags {
		args = append(args, flag.String())
	}
	args = append(args, targetArgs...)
	args = append(args, cl.Args...)
	return args
}

// SetIsolatedFlags updates cl to include command-line flags to run in an
// isolated environment.
//
// The supplied arguments have several Python isolation flags prepended to them
// to remove environmental factors such as:
//	- The user's "site.py".
//	- The current PYTHONPATH environment variable.
//	- Compiled ".pyc/.pyo" files.
func (cl *CommandLine) SetIsolatedFlags() {
	cl.AddInterpreterSingleFlag('B') // Don't compile "pyo" binaries.
	cl.AddInterpreterSingleFlag('E') // Don't use PYTHON* environment variables.
	cl.AddInterpreterSingleFlag('s') // Don't use user 'site.py'.
}

// AddInterpreterFlag adds an interpreter flag to cl if it's not already
// present.
func (cl *CommandLine) AddInterpreterFlag(flag CommandLineFlag) {
	for _, f := range cl.Flags {
		if f == flag {
			return
		}
	}
	cl.Flags = append(cl.Flags, flag)
}

// AddInterpreterSingleFlag adds a single no-argument interpreter flag to cl
// if it's not already specified.
func (cl *CommandLine) AddInterpreterSingleFlag(flag rune) {
	cl.AddInterpreterFlag(CommandLineFlag{Flag: string(flag)})
}

// RemoveInterpreterFlag removes all instances of the specified flag
// from the interpreter command line.
func (cl *CommandLine) RemoveInterpreterFlag(flag rune) (found bool) {
	flagStr := string(flag)
	newFlags := cl.Flags[:0]
	for _, f := range cl.Flags {
		if f.Flag != flagStr {
			newFlags = append(newFlags, f)
		} else {
			found = true
		}
	}
	cl.Flags = newFlags
	return
}

// Clone returns an independent deep copy of cl.
func (cl *CommandLine) Clone() *CommandLine {
	return &CommandLine{
		Target: cl.Target,
		Flags:  append([]CommandLineFlag(nil), cl.Flags...),
		Args:   append([]string(nil), cl.Args...),
	}
}

// parseSingleFlag parses a single flag from inFlag.
//
// inFlag is the flag string, minus the leading "-". inArgs is the set of
// arguments that follows it. Returns any unparsed remainder of inFlag and
// any unconsumed arguments.
func (cl *CommandLine) parseSingleFlag(inFlag string, inArgs []string) (
	flag string, args []string, err error) {

	flag, args = inFlag, inArgs

	twoVarType := func() (val string, err error) {
		switch {
		case len(flag) > 0:
			val, flag = flag, ""
			return
		case len(args) == 0:
			err = errors.New("two-value flag missing second value")
			return
		default:
			// Consume the argument.
			val, args = args[0], args[1:]
			return
		}
	}

	// Consume the first character from flag into "r". "flag" is the remainder.
	r, l := utf8.DecodeRuneInString(flag)
	if r == utf8.RuneError {
		err = errors.Reason("invalid rune in flag").Err()
		return
	}
	flag = flag[l:]

	// Is this a combined flag/value (e.g., -c'paoskdpo') ?
	switch r {
	// Two-variable execution flags.
	case 'c':
		var val string
		if val, err = twoVarType(); err != nil {
			return
		}
		cl.Target = CommandTarget{val}

	case 'm':
		var val string
		if val, err = twoVarType(); err != nil {
			return
		}
		cl.Target = ModuleTarget{val}

	case 'Q', 'W', 'X':
		// Random two-argument Python flags.
		var val string
		if val, err = twoVarType(); err != nil {
			return
		}
		cl.Flags = append(cl.Flags, CommandLineFlag{string(r), val})

	default:
		// One-argument Python flags. If there are more characters in "flag",
		// don't consume the entire flag; instead, replace it with the remainder
		// for subsequent parses. This handles cases like "-vvc <script>".
		cl.Flags = append(cl.Flags, CommandLineFlag{string(r), ""})
	}

	return
}

// ParseCommandLine parses Python command-line arguments and returns a
// structured representation.
func ParseCommandLine(args []string) (*CommandLine, error) {
	noTarget := NoTarget{}

	cl := CommandLine{
		Target: noTarget,
	}
	var (
		i            = 0
		canHaveFlags = true
	)
	for len(args) > 0 {
		// Stop parsing after we have a target, as Python does.
		if cl.Target != noTarget {
			break
		}

		// Consume the next argument.
		arg := args[0]
		args = args[1:]
		i++

		if arg == "-" {
			// "-" instructs Python to load the script from STDIN.
			cl.Target = ScriptTarget{
				Path: "-",
			}
			continue
		}

		isFlag := false
		if canHaveFlags {
			arg, isFlag = trimPrefix(arg, "-")
		}

		if !isFlag {
			// The first positional argument is the path to the script, and all
			// subsequent arguments are script arguments.
			cl.Target = ScriptTarget{
				Path: arg,
			}
			continue
		}

		// If we encounter "--", that marks the end of interpreter flag parsing.
		if arg == "-" {
			canHaveFlags = false
			cl.Flags = append(cl.Flags, CommandLineFlag{"-", ""})
			continue
		}

		// Parse this flag and any remainder.
		for len(arg) > 0 {
			var err error
			arg, args, err = cl.parseSingleFlag(arg, args)
			if err != nil {
				return nil, errors.Annotate(err, "failed to parse Python flag #%d", i).
					InternalReason("arg(%q)", arg).Err()
			}
		}
	}

	// The remainder of arguments are for the script.
	cl.Args = append([]string(nil), args...)
	return &cl, nil
}

func trimPrefix(v, pfx string) (string, bool) {
	if strings.HasPrefix(v, pfx) {
		return v[len(pfx):], true
	}
	return v, false
}
