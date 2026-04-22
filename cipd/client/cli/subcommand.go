// Copyright 2026 The LUCI Authors.
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

package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/term"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/cipd/client/cipd/ui"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// This is a killswitch that disables the fancy terminal progress bar UI in case
// it has some fatal bugs or a user has aversion towards it.
//
// Note that cipd.Client doesn't know anything about the UI implementation and
// thus this env var is defined here rather than in cipd/client.go like other
// env vars.
const envSimpleTerminalUI = "CIPD_SIMPLE_TERMINAL_UI"

////////////////////////////////////////////////////////////////////////////////
// Common subcommand functions.

// cipdSubcommand is a base of all CIPD subcommands. It defines some common
// flags, such as logging and JSON output parameters.
type cipdSubcommand struct {
	subcommands.CommandRunBase

	jsonOutput string
	logConfig  logging.Config

	// TODO(dnj): Remove "verbose" flag once all current invocations of it are
	// cleaned up and rolled out, as it is now deprecated in favor of "logConfig".
	verbose bool
}

// ModifyContext implements cli.ContextModificator.
func (c *cipdSubcommand) ModifyContext(ctx context.Context) context.Context {
	if c.verbose {
		ctx = logging.SetLevel(ctx, logging.Debug)
	} else {
		ctx = c.logConfig.Set(ctx)
	}

	// Give a lever to turn off the fancy UI if necessary.
	useSimpleUI := environ.FromCtx(ctx).Get(envSimpleTerminalUI) == "1"

	// If writing to a real terminal (rather than redirecting to a file) and not
	// running at a non-default logging level, use a fancy UI with progress bars.
	// It is more human readable, but doesn't preserve details of all operations
	// in the terminal output.
	if !useSimpleUI && logging.GetLevel(ctx) == logging.Info && term.IsTerminal(int(os.Stderr.Fd())) {
		ctx = ui.SetImplementation(ctx, &ui.FancyImplementation{Out: os.Stderr})
	}

	return ctx
}

// registerBaseFlags registers common flags used by all subcommands.
func (c *cipdSubcommand) registerBaseFlags() {
	// Minimum default logging level is Info. This accommodates subcommands that
	// don't explicitly set the log level, resulting in the zero value (Debug).
	if c.logConfig.Level < logging.Info {
		c.logConfig.Level = logging.Info
	}

	c.Flags.StringVar(&c.jsonOutput, "json-output", "", "A `path` to write operation results to.")
	c.Flags.BoolVar(&c.verbose, "verbose", false, "Enable more logging (deprecated, use -log-level=debug).")
	c.logConfig.AddFlags(&c.Flags)
}

// checkArgs checks command line args.
//
// It ensures all required positional and flag-like parameters are set.
// Returns true if they are, or false (and prints to stderr) if not.
func (c *cipdSubcommand) checkArgs(args []string, minPosCount, maxPosCount int) bool {
	// Check number of expected positional arguments.
	if maxPosCount == 0 && len(args) != 0 {
		c.printError(makeCLIError("unexpected arguments %v", args))
		return false
	}
	if len(args) < minPosCount || (maxPosCount >= 0 && len(args) > maxPosCount) {
		var err error
		if minPosCount == maxPosCount {
			err = makeCLIError("expecting %d positional argument, got %d instead", minPosCount, len(args))
		} else {
			if maxPosCount >= 0 {
				err = makeCLIError(
					"expecting from %d to %d positional arguments, got %d instead",
					minPosCount, maxPosCount, len(args))
			} else {
				err = makeCLIError(
					"expecting at least %d positional arguments, got %d instead",
					minPosCount, len(args))
			}
		}
		c.printError(err)
		return false
	}

	// Check required unset flags.
	unset := []*flag.Flag{}
	c.Flags.VisitAll(func(f *flag.Flag) {
		if strings.HasPrefix(f.DefValue, "<") && f.Value.String() == f.DefValue {
			unset = append(unset, f)
		}
	})
	if len(unset) != 0 {
		missing := make([]string, len(unset))
		for i, f := range unset {
			missing[i] = f.Name
		}
		c.printError(makeCLIError("missing required flags: %v", missing))
		return false
	}

	return true
}

// printError prints error to stderr (recognizing cliErrorTag).
func (c *cipdSubcommand) printError(err error) {
	if cliErrorTag.In(err) {
		fmt.Fprintf(os.Stderr, "Bad command line: %s.\n\n", err)
		c.Flags.Usage()
		return
	}

	if merr, _ := err.(errors.MultiError); len(merr) != 0 {
		fmt.Fprintln(os.Stderr, "Errors:")
		for _, err := range merr {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "Error: %s.\n", err)
}

// writeJSONOutput writes result to JSON output file. It returns original error
// if it is non-nil.
func (c *cipdSubcommand) writeJSONOutput(result any, err error) error {
	// -json-output flag wasn't specified.
	if c.jsonOutput == "" {
		return err
	}

	// Prepare the body of the output file.
	var body struct {
		Error        string           `json:"error,omitempty"`         // human-readable message
		ErrorCode    cipderr.Code     `json:"error_code,omitempty"`    // error code enum, omitted on success
		ErrorDetails *cipderr.Details `json:"error_details,omitempty"` // structured error details
		Result       any              `json:"result,omitempty"`
	}
	if err != nil {
		body.Error = err.Error()
		body.ErrorCode = cipderr.ToCode(err)
		body.ErrorDetails = cipderr.ToDetails(err)
	}
	body.Result = result
	out, e := json.MarshalIndent(&body, "", "  ")
	if e != nil {
		if err == nil {
			err = e
		} else {
			fmt.Fprintf(os.Stderr, "Failed to serialize JSON output: %s\n", e)
		}
		return err
	}

	e = os.WriteFile(c.jsonOutput, out, 0666)
	if e != nil {
		if err == nil {
			err = e
		} else {
			fmt.Fprintf(os.Stderr, "Failed write JSON output to %s: %s\n", c.jsonOutput, e)
		}
		return err
	}

	return err
}

// done is called as a last step of processing a subcommand. It dumps command
// result (or error) to JSON output file, prints error message and generates
// process exit code.
func (c *cipdSubcommand) done(result any, err error) int {
	err = c.writeJSONOutput(result, err)
	if err != nil {
		c.printError(err)
		return 1
	}
	return 0
}

// doneWithPins is a handy shortcut that prints a pinInfo slice and
// deduces process exit code based on presence of errors there.
//
// This just calls through to doneWithPinMap.
func (c *cipdSubcommand) doneWithPins(pins []pinInfo, err error) int {
	return c.doneWithPinMap(map[string][]pinInfo{"": pins}, err)
}

// doneWithPinMap is a handy shortcut that prints the subdir->pinInfo map and
// deduces process exit code based on presence of errors there.
func (c *cipdSubcommand) doneWithPinMap(pins map[string][]pinInfo, err error) int {
	// If have an overall (not pin-specific error), print it before pin errors.
	if err != nil {
		c.printError(err)
	}
	printPinsAndError(pins)

	// If have no overall error, hoist all pin errors up top, so we have a final
	// overall error for writeJSONOutput, otherwise it may look as if the call
	// succeeded.
	if err == nil {
		var merr errors.MultiError
		for _, pinSlice := range pins {
			for _, pin := range pinSlice {
				if pin.err != nil {
					merr = append(merr, pin.err)
				}
			}
		}
		if len(merr) != 0 {
			err = merr
		}
	}

	// Dump all this to JSON. Note we don't use done(...) to avoid printing the
	// error again.
	if c.writeJSONOutput(pins, err) != nil {
		return 1
	}
	return 0
}

// cliErrorTag is used to tag errors related to CLI.
var cliErrorTag = errtag.Make("CIPD CLI error", true)

// makeCLIError returns a new error tagged with cliErrorTag and BadArgument.
func makeCLIError(msg string, args ...any) error {
	return cliErrorTag.Apply(cipderr.BadArgument.Apply(errors.Fmt(msg, args...)))
}

// pinInfo contains information about single package pin inside some site root,
// or an error related to it. It is passed through channels when running batch
// operations and dumped to JSON results file in doneWithPins.
type pinInfo struct {
	// Pkg is package name. Always set.
	Pkg string `json:"package"`
	// Pin is not nil if pin related operation succeeded. It contains instanceID.
	Pin *common.Pin `json:"pin,omitempty"`
	// Platform is set by 'ensure-file-verify' to a platform for this pin.
	Platform string `json:"platform,omitempty"`
	// Tracking is what ref is being tracked by that package in the site root.
	Tracking string `json:"tracking,omitempty"`
	// Error is not empty if pin related operation failed. Pin is nil in that case.
	Error string `json:"error,omitempty"`
	// ErrorCode is an enumeration with possible error conditions.
	ErrorCode cipderr.Code `json:"error_code,omitempty"`
	// ErrorDetails are structured error details.
	ErrorDetails *cipderr.Details `json:"error_details,omitempty"`

	// The original annotated error.
	err error `json:"-"`
}
