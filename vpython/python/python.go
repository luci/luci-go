// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package python

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/luci/luci-go/common/errors"
)

// Error is a Python error return code.
type Error int

func (err Error) Error() string {
	return fmt.Sprintf("python interpreter returned non-zero error: %d", err)
}

// TargetType is an enumeration of possible Python invocation targets.
//
// Targets are identified by parsing a Python command-line using
// ParseCommandLine.
type TargetType int

const (
	// TargetNone means no Python target (i.e., interactive).
	TargetNone TargetType = iota
	// TargetScript is a Python executable script target.
	TargetScript
	// TargetCommand runs a command-line string (-c ...).
	TargetCommand
	// TargetModule runs a Python module (-m ...).
	TargetModule
)

// CommandLine is a parsed Python command-line.
//
// CommandLine can be parsed from arguments via ParseCommandLine.
type CommandLine struct {
	// Type is the Python target type.
	Type TargetType
	// Value is the execution value for Type.
	Value string

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
	for i < len(args) && cmd.Type == TargetNone {
		arg := args[i]
		i++

		flag, has := trimPrefix(arg, "-")
		if !has {
			// Non-flag argument, so path to script.
			cmd.Type = TargetScript
			cmd.Value = flag
			break
		}

		// -<flag>
		if len(flag) == 0 {
			// "-" instructs Python to load the script from STDIN.
			cmd.Type, cmd.Value = TargetScript, "-"
			break
		}

		// Extract the flag value. -f<lag>
		r, l := utf8.DecodeRuneInString(flag)
		if r == utf8.RuneError {
			err = errors.Reason("invalid rune in flag #%(index)d").D("index", i).Err()
			return
		}

		// Is this a combined flag/value (e.g., -c'paoskdpo') ?
		flag = flag[l:]
		twoVarType := func() error {
			if len(flag) > 0 {
				cmd.Value = flag
				return nil
			}

			if i >= len(args) {
				return errors.Reason("two-value flag -%(flag)c missing second value at %(index)d").
					D("flag", r).
					D("index", i).
					Err()
			}
			cmd.Value = args[i]
			i++
			return nil
		}

		switch r {
		// Two-variable execution flags.
		case 'c':
			cmd.Type = TargetCommand
			if err = twoVarType(); err != nil {
				return
			}

		case 'm':
			cmd.Type = TargetModule
			if err = twoVarType(); err != nil {
				return
			}

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
