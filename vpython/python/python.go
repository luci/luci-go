// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package python

import (
	"strings"
	"unicode/utf8"

	"github.com/luci/luci-go/common/errors"
)

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
	// implementsTarget is an internal method used to constrain Target
	// implementations to internal packages.
	implementsTarget()
}

// NoTarget is a Target implementation indicating no Python target (i.e.,
// interactive).
type NoTarget struct{}

func (st NoTarget) implementsTarget() {}

// ScriptTarget is a Python executable script target.
type ScriptTarget struct {
	// Path is the path to the script that is being invoked.
	//
	// This may be "-", indicating that the script is being read from STDIN.
	Path string
}

func (st ScriptTarget) implementsTarget() {}

// CommandTarget is a Target implementation for a command-line string
// (-c ...).
type CommandTarget struct {
	// Command is the command contents.
	Command string
}

func (st CommandTarget) implementsTarget() {}

// ModuleTarget is a Target implementating indicating a Python module (-m ...).
type ModuleTarget struct {
	// Module is the name of the target module.
	Module string
}

func (st ModuleTarget) implementsTarget() {}

// CommandLine is a parsed Python command-line.
//
// CommandLine can be parsed from arguments via ParseCommandLine.
type CommandLine struct {
	// Target is the Python target type.
	Target Target

	// Flags are flags to the Python interpreter.
	Flags []string
	// Args are arguments passed to the Python script.
	Args []string
}

// ParseCommandLine parses Python command-line arguments and returns a
// structured representation.
func ParseCommandLine(args []string) (cmd CommandLine, err error) {
	flags := 0
	i := 0
	for i < len(args) && cmd.Target == nil {
		arg := args[i]
		i++

		flag, has := trimPrefix(arg, "-")
		if !has {
			// Non-flag argument, so path to script.
			cmd.Target = ScriptTarget{
				Path: flag,
			}
			break
		}

		// -<flag>
		if len(flag) == 0 {
			// "-" instructs Python to load the script from STDIN.
			cmd.Target = ScriptTarget{
				Path: "-",
			}
			break
		}

		// Extract the flag Target. -f<lag>
		r, l := utf8.DecodeRuneInString(flag)
		if r == utf8.RuneError {
			err = errors.Reason("invalid rune in flag #%(index)d").D("index", i).Err()
			return
		}

		// Is this a combined flag/value (e.g., -c'paoskdpo') ?
		flag = flag[l:]
		twoVarType := func() (string, error) {
			if len(flag) > 0 {
				return flag, nil
			}

			if i >= len(args) {
				return "", errors.Reason("two-value flag -%(flag)c missing second value at %(index)d").
					D("flag", r).
					D("index", i).
					Err()
			}

			value := args[i]
			i++
			return value, nil
		}

		switch r {
		// Two-variable execution flags.
		case 'c':
			var target CommandTarget
			if target.Command, err = twoVarType(); err != nil {
				return
			}
			cmd.Target = target

		case 'm':
			var target ModuleTarget
			if target.Module, err = twoVarType(); err != nil {
				return
			}
			cmd.Target = target

		case 'Q', 'W', 'X':
			// Random two-argument Python flags.
			if len(flag) == 0 {
				flags++
				i++
			}
			fallthrough

		default:
			// One-argument Python flags.
			flags++
		}
	}

	if i > len(args) {
		err = errors.New("truncated two-variable argument")
		return
	}

	// If no target was specified, use NoTarget.
	if cmd.Target == nil {
		cmd.Target = NoTarget{}
	}

	cmd.Flags = args[:flags]
	cmd.Args = args[i:]
	return
}

func trimPrefix(v, pfx string) (string, bool) {
	if strings.HasPrefix(v, pfx) {
		return v[len(pfx):], true
	}
	return v, false
}
