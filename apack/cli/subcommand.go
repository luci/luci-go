// Copyright 2019 The LUCI Authors.
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

// Package cli contains command line interface for lucicfg tool.
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/logging"
)

// subcommand is a base of all subcommands.
//
// It defines some common flags, such as logging and JSON output parameters,
// and some common methods to report errors and dump JSON output.
//
// It's Init() method should be called from within CommandRun to register
// base flags.
type subcommand struct {
	subcommands.CommandRunBase

	params    *Parameters    // whatever was passed to Init
	logConfig logging.Config // for -log-level, used by ModifyContext
}

// Init registers common flags.
func (c *subcommand) Init(params Parameters) {
	c.params = &params
	c.logConfig.Level = logging.Info

	c.logConfig.AddFlags(&c.Flags)
}

// CheckArgs checks command line args.
//
// It ensures all required positional and flag-like parameters are set. Setting
// maxPosCount to -1 indicates there is unbounded number of positional arguments
// allowed.
//
// Returns true if they are, or false (and prints to stderr) if not.
func (c *subcommand) CheckArgs(args []string, minPosCount, maxPosCount int) bool {
	// Check number of expected positional arguments.
	if len(args) < minPosCount || (maxPosCount >= 0 && len(args) > maxPosCount) {
		var err error
		switch {
		case maxPosCount == 0:
			err = newCLIError("unexpected arguments %v", args)
		case minPosCount == maxPosCount:
			err = newCLIError("expecting %d positional argument, got %d instead", minPosCount, len(args))
		case maxPosCount >= 0:
			err = newCLIError(
				"expecting from %d to %d positional arguments, got %d instead",
				minPosCount, maxPosCount, len(args))
		default:
			err = newCLIError(
				"expecting at least %d positional arguments, got %d instead",
				minPosCount, len(args))
		}
		c.printError(err)
		return false
	}

	// Check required unset flags. A flag is considered required if its default
	// value has form '<...>'.
	unset := []*flag.Flag{}
	c.Flags.VisitAll(func(f *flag.Flag) {
		d := f.DefValue
		if strings.HasPrefix(d, "<") && strings.HasSuffix(d, ">") && f.Value.String() == d {
			unset = append(unset, f)
		}
	})
	if len(unset) != 0 {
		missing := make([]string, len(unset))
		for i, f := range unset {
			missing[i] = f.Name
		}
		c.printError(newCLIError("missing required flags: %v", missing))
		return false
	}

	return true
}

// ModifyContext implements cli.ContextModificator.
func (c *subcommand) ModifyContext(ctx context.Context) context.Context {
	return c.logConfig.Set(ctx)
}

// done is called as the last step of processing a subcommand.
func (c *subcommand) done(err error) int {
	if err != nil {
		c.printError(err)
		return 1
	}
	return 0
}

// printError prints an error to stderr.
//
// Recognizes various sorts of known errors and reports the appropriately.
func (c *subcommand) printError(err error) {
	if _, ok := err.(commandLineError); ok {
		fmt.Fprintf(os.Stderr, "Bad command line: %s.\n\n", err)
		c.Flags.Usage()
	} else {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
	}
}

// commandLineError is used to tag errors related to command line arguments.
//
// subcommand.Done(..., err) will print the usage string if it finds such error.
type commandLineError struct {
	error
}

// newCLIError returns new commandLineError.
func newCLIError(msg string, args ...interface{}) error {
	return commandLineError{fmt.Errorf(msg, args...)}
}

// missingFlagError is CommandLineError about a missing flag.
func missingFlagError(flag string) error {
	return newCLIError("%s is required", flag)
}
