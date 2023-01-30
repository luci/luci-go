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
//   - NoTarget
//   - ScriptTarget
//   - CommandTarget
//   - ModuleTarget
type Target interface {
	// buildArgsForTarget returns the arguments to pass to the interpreter to
	// invoke this target.
	buildArgsForTarget() []string
	// followsFlagSeparator returns true if this target follows the command-line
	// flag separator (if present). This will be false for all flag arguments,
	// since flags must precede the separator.
	//
	//	- true: python <flags> -- <script> ...
	//	- false: python <flags> <script> -- ...
	followsFlagSeparator() bool
}

// NoTarget is a Target implementation indicating no Python target (i.e.,
// interactive).
type NoTarget struct{}

func (NoTarget) buildArgsForTarget() []string { return nil }
func (NoTarget) followsFlagSeparator() bool   { return false }

// ScriptTarget is a Python executable script target.
type ScriptTarget struct {
	// Path is the path to the script that is being invoked.
	//
	// This may be "-", indicating that the script is being read from STDIN.
	Path string
	// FollowsSeparator is true if the script argument follows the flag separator.
	FollowsSeparator bool
}

func (t ScriptTarget) buildArgsForTarget() []string { return []string{t.Path} }
func (t ScriptTarget) followsFlagSeparator() bool   { return t.FollowsSeparator }

// CommandTarget is a Target implementation for a command-line string
// (-c ...).
type CommandTarget struct {
	// Command is the command contents.
	Command string
}

func (t CommandTarget) buildArgsForTarget() []string { return []string{"-c", t.Command} }
func (CommandTarget) followsFlagSeparator() bool     { return false }

// ModuleTarget is a Target implementing indicating a Python module (-m ...).
type ModuleTarget struct {
	// Module is the name of the target module.
	Module string
}

func (t ModuleTarget) buildArgsForTarget() []string { return []string{"-m", t.Module} }
func (ModuleTarget) followsFlagSeparator() bool     { return false }

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
	// FlagSeparator, if true, means that a "--" flag separator, which separates
	// the interpreter's flags from its positional arguments, was found.
	FlagSeparator bool
	// Args are arguments passed to the Python script.
	Args []string
}

// BuildArgs returns an array of Python interpreter arguments for cl.
func (cl *CommandLine) BuildArgs() []string {
	targetArgs := cl.Target.buildArgsForTarget()
	args := make([]string, 0, len(cl.Flags)+1+len(targetArgs)+len(cl.Args))
	for _, flag := range cl.Flags {
		args = append(args, flag.String())
	}

	var flagSeparator []string
	if cl.FlagSeparator {
		flagSeparator = []string{"--"}
	}

	// If our target is specified as a flag, we need to emit it before the flag
	// separator. If our target is specified as a positional argument (e.g.,
	// CommandTarget), we can emit it on either side.
	if !cl.Target.followsFlagSeparator() {
		args = append(args, targetArgs...)
		args = append(args, flagSeparator...)
	} else {
		args = append(args, flagSeparator...)
		args = append(args, targetArgs...)
	}

	args = append(args, cl.Args...)
	return args
}

// AddFlag adds an interpreter flag to cl if it's not already present.
func (cl *CommandLine) AddFlag(flag CommandLineFlag) {
	if strings.HasPrefix(flag.Flag, "-") {
		panic("flag must not begin with '-'")
	}
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
	// Consume the first character from flag into "r". "flag" is the remainder.
	r, l := utf8.DecodeRuneInString(fs.flag)
	if r == utf8.RuneError {
		return errors.Reason("invalid rune in flag").Err()
	}
	fs.flag = fs.flag[l:]

	// Retrieve the value for a non-binary flag. This mutates the flag state to
	// consume that value.
	getFlagValue := func() (val string, err error) {
		switch {
		case len(fs.flag) > 0:
			// Combined flag/value (e.g., -c'paoskdpo')
			val, fs.flag = fs.flag, ""
		case len(fs.args) == 0:
			err = errors.New("two-value flag missing second value")
		default:
			// Flag value is in subsequent argument (e.g., "-c 'paoskdpo'").
			// Consume the argument.
			val, fs.args = fs.args[0], fs.args[1:]
		}
		return
	}

	// Some cases will set this to true if `r` is determined to just be a no-value
	// single-character flag
	isSingleCharFlag := false

	switch r {
	case 'c':
		// Inline command target.
		val, err := getFlagValue()
		if err != nil {
			return err
		}
		cl.Target = CommandTarget{val}

	case 'm':
		// Python module target.
		val, err := getFlagValue()
		if err != nil {
			return err
		}
		cl.Target = ModuleTarget{val}

	case 'Q', 'W', 'X':
		// Two-argument Python flags.
		val, err := getFlagValue()
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

		// Single "O", do normal single-flag parsing.
		isSingleCharFlag = true

	case '-':
		// handle the case of "--version", which is an atypical many-character flag.
		if fs.flag == "version" {
			fs.flag = ""
			cl.Flags = append(cl.Flags, CommandLineFlag{"-version", ""})
			break
		}

		// Not sure what this could be, but fall through none the less.
		isSingleCharFlag = true

	default:
		isSingleCharFlag = true
	}

	if isSingleCharFlag {
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
	i := 0
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
		if !cl.FlagSeparator {
			arg, isFlag = trimPrefix(arg, "-")
		}

		if !isFlag {
			// The first positional argument is the path to the script, and all
			// subsequent arguments are script arguments.
			cl.Target = ScriptTarget{
				Path:             arg,
				FollowsSeparator: cl.FlagSeparator,
			}
			continue
		}

		// Note that at this point we've trimmed the preceding "-" from arg, so
		// this is really "--". If we encounter "--" that marks the end of
		// interpreter flag parsing; everything hereafter is considered positional
		// to the interpreter.
		if arg == "-" {
			cl.FlagSeparator = true
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
