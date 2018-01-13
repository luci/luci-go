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

// String returns a string representation of this flag, which is a command-line
// suitable representation of its value.
func (f *CommandLineFlag) String() string {
	return fmt.Sprintf("-%s%s", f.Flag, f.Arg)
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

// parsedFlagState is the current state of a parsed flag block. It is advanced
// in CommandLine's parseSingleFlag as flags are parsed.
type parsedFlagState struct {
	// flag is the current flag block. It does not include the preceding "-".
	//
	// If a block is single, e.g., "-w", it will contain "w".
	// If a block contains multiple flags, e.g, "-vvv", it will contain "vvv".
	flag string
	// args is the remainder of args following the flag block. It is used when
	// a multi-argument flag does not include a conjoined data block.
	//
	// For example, "-Wall" has flag "W", value "all". ["-W", "all"], parses
	// identically, but requires the argument after the "-W" to resolve.
	args []string
}

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
	cl.AddSingleFlag("B") // Don't compile "pyo" binaries.
	cl.AddSingleFlag("E") // Don't use PYTHON* environment variables.
	cl.AddSingleFlag("s") // Don't use user 'site.py'.
}

// AddFlag adds an interpreter flag to cl if it's not already present.
func (cl *CommandLine) AddFlag(flag CommandLineFlag) {
	for _, f := range cl.Flags {
		if f == flag {
			return
		}
	}
	cl.Flags = append(cl.Flags, flag)
}

// AddSingleFlag adds a single no-argument interpreter flag to cl
// if it's not already specified.
func (cl *CommandLine) AddSingleFlag(flag string) {
	cl.AddFlag(CommandLineFlag{Flag: flag})
}

// RemoveFlagMatch removes all instances of flags that match the selection
// function.
//
// matchFn is a function that accepts a candidate flag and returns true if it
// should be removed, false if it should not.
func (cl *CommandLine) RemoveFlagMatch(matchFn func(CommandLineFlag) bool) (found bool) {
	newFlags := cl.Flags[:0]
	for _, f := range cl.Flags {
		if !matchFn(f) {
			newFlags = append(newFlags, f)
		} else {
			found = true
		}
	}
	cl.Flags = newFlags
	return
}

// RemoveFlag removes all instances of the specified flag from the interpreter
// command line.
func (cl *CommandLine) RemoveFlag(flag CommandLineFlag) (found bool) {
	return cl.RemoveFlagMatch(func(f CommandLineFlag) bool { return f == flag })
}

// RemoveAllFlag removes all instances of the specified flag from the interpreter
// command line.
func (cl *CommandLine) RemoveAllFlag(flag string) (found bool) {
	return cl.RemoveFlagMatch(func(f CommandLineFlag) bool { return f.Flag == flag })
}

// Clone returns an independent deep copy of cl.
func (cl *CommandLine) Clone() *CommandLine {
	return &CommandLine{
		Target: cl.Target,
		Flags:  append([]CommandLineFlag(nil), cl.Flags...),
		Args:   append([]string(nil), cl.Args...),
	}
}

// parseSingleFlag parses a single flag from a state.
func (cl *CommandLine) parseSingleFlag(fs *parsedFlagState) error {
	twoVarType := func() (val string, err error) {
		switch {
		case len(fs.flag) > 0:
			val, fs.flag = fs.flag, ""
			return
		case len(fs.args) == 0:
			err = errors.New("two-value flag missing second value")
			return
		default:
			// Consume the argument.
			val, fs.args = fs.args[0], fs.args[1:]
			return
		}
	}

	// Consume the first character from flag into "r". "flag" is the remainder.
	r, l := utf8.DecodeRuneInString(fs.flag)
	if r == utf8.RuneError {
		return errors.Reason("invalid rune in flag").Err()
	}
	fs.flag = fs.flag[l:]

	// Is this a combined flag/value (e.g., -c'paoskdpo') ?
	switch r {
	// Two-variable execution flags.
	case 'c':
		val, err := twoVarType()
		if err != nil {
			return err
		}
		cl.Target = CommandTarget{val}

	case 'm':
		val, err := twoVarType()
		if err != nil {
			return err
		}
		cl.Target = ModuleTarget{val}

	case 'Q', 'W', 'X':
		// Two-argument Python flags.
		val, err := twoVarType()
		if err != nil {
			return err
		}
		cl.Flags = append(cl.Flags, CommandLineFlag{string(r), val})

	case 'O':
		// Handle the case of the odd flag "-OO", which parses as a single flag.
		var has bool
		if fs.flag, has = trimPrefix(fs.flag, "O"); has {
			cl.Flags = append(cl.Flags, CommandLineFlag{"OO", ""})
			break
		}

		// Single "O", fall through to single-flag parsing.
		fallthrough

	default:
		// One-argument Python flags. If there are more characters in "flag",
		// don't consume the entire flag; instead, replace it with the remainder
		// for subsequent parses. This handles cases like "-vvc <script>".
		cl.Flags = append(cl.Flags, CommandLineFlag{string(r), ""})
	}

	return nil
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

		// Note that ta this point we've trimmed the preceding "-" from arg, so
		// this is really "--". If we encounter "--" that marks the end of
		// interpreter flag parsing; everything hereafter is considered positional
		// to the interpreter.
		if arg == "-" {
			canHaveFlags = false
			cl.Flags = append(cl.Flags, CommandLineFlag{"-", ""})
			continue
		}

		// Parse this flag and any remainder.
		fs := parsedFlagState{
			flag: arg,
			args: args,
		}
		for len(fs.flag) > 0 {
			if err := cl.parseSingleFlag(&fs); err != nil {
				return nil, errors.Annotate(err, "failed to parse Python flag #%d", i).
					InternalReason("arg(%q)", arg).Err()
			}
		}
		args = fs.args
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
