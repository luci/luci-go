// Copyright 2018 The LUCI Authors.
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

// Package base contains code shared by other CLI subpackages.
package base

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg"
)

// CommandLineError is used to tag errors related to command line arguments.
//
// Subcommand.PrintError() will print the usage string if it finds such error.
type CommandLineError struct {
	error
}

// NewCLIError returns new CommandLineError.
func NewCLIError(msg string, args ...interface{}) error {
	return CommandLineError{fmt.Errorf(msg, args...)}
}

// Parameters can be used to customize CLI defaults.
type Parameters struct {
	AuthOptions       auth.Options // mostly for client ID and client secret
	ConfigServiceHost string       // e.g. "luci-config.appspot.com"
}

// Subcommand is a base of all subcommands.
//
// It defines some common flags, such as logging and JSON output parameters,
// and some common methods to report errors and dump JSON output.
//
// It's Init() method should be called from within CommandRun to register
// base flags.
type Subcommand struct {
	subcommands.CommandRunBase

	makesRPCs bool // set in init(...)

	jsonOutput  string
	logConfig   logging.Config
	authFlags   authcli.Flags // only for commands that do RPCs
	serviceHost string        // only for commands that do RPCs
}

// ModifyContext implements cli.ContextModificator.
func (c *Subcommand) ModifyContext(ctx context.Context) context.Context {
	return c.logConfig.Set(ctx)
}

// Init registers common flags.
//
// If makesRPCs is true, will register flags related to authentication and RPCs.
func (c *Subcommand) Init(params Parameters, makesRPCs bool) {
	c.makesRPCs = makesRPCs
	c.logConfig.Level = logging.Info // default to Info level

	c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to.")
	c.logConfig.AddFlags(&c.Flags)

	if makesRPCs {
		c.authFlags.Register(&c.Flags, params.AuthOptions)
		c.Flags.StringVar(&c.serviceHost, "config-service-host", params.ConfigServiceHost, "Hostname of a LUCI config service to send RPCs to.")
	}
}

// CheckArgs checks command line args.
//
// It ensures all required positional and flag-like parameters are set. Setting
// maxPosCount to -1 indicates there is unbounded number of positional arguments
// allowed.
//
// Returns true if they are, or false (and prints to stderr) if not.
func (c *Subcommand) CheckArgs(args []string, minPosCount, maxPosCount int) bool {
	// Check number of expected positional arguments.
	if len(args) < minPosCount || (maxPosCount >= 0 && len(args) > maxPosCount) {
		var err error
		switch {
		case maxPosCount == 0:
			err = NewCLIError("unexpected arguments %v", args)
		case minPosCount == maxPosCount:
			err = NewCLIError("expecting %d positional argument, got %d instead", minPosCount, len(args))
		case maxPosCount >= 0:
			err = NewCLIError(
				"expecting from %d to %d positional arguments, got %d instead",
				minPosCount, maxPosCount, len(args))
		default:
			err = NewCLIError(
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
		c.printError(NewCLIError("missing required flags: %v", missing))
		return false
	}

	return true
}

// configService returns a wrapper around LUCI Config API.
//
// It is ready for making authenticated RPCs.
func (c *Subcommand) ConfigService(ctx context.Context) (*config.Service, error) {
	if !c.makesRPCs {
		panic("the subcommand wasn't declared as one making RPCs")
	}

	authOpts, err := c.authFlags.Options()
	if err != nil {
		return nil, err
	}
	client, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		return nil, err
	}

	svc, err := config.New(client)
	if err != nil {
		return nil, err
	}
	svc.BasePath = fmt.Sprintf("https://%s/_ah/api/config/v1/", c.serviceHost)
	svc.UserAgent = lucicfg.UserAgent
	return svc, nil
}

// Done is called as the last step of processing a subcommand.
//
// It dumps the command result (or an error) to the JSON output file, prints
// the error message and generates the process exit code.
func (c *Subcommand) Done(result interface{}, err error) int {
	err = c.writeJSONOutput(result, err)
	if err != nil {
		c.printError(err)
		return 1
	}
	return 0
}

// printError prints an error to stderr.
//
// Recognizes various sorts of known errors and reports the appropriately.
func (c *Subcommand) printError(err error) {
	if _, ok := err.(CommandLineError); ok {
		fmt.Fprintf(os.Stderr, "Bad command line: %s.\n\n", err)
		c.Flags.Usage()
		return
	}

	if err == auth.ErrLoginRequired {
		fmt.Fprintf(os.Stderr, "You need to login first by running:\nlucicfg auth-login\n")
		return
	}

	errors.WalkLeaves(err, func(err error) bool {
		if bt, ok := err.(lucicfg.BacktracableError); ok {
			fmt.Fprintf(os.Stderr, "%s\n\n", bt.Backtrace())
		} else {
			fmt.Fprintf(os.Stderr, "%s\n\n", err)
		}
		return true
	})
}

// WriteJSONOutput writes result to JSON output file (if -json-output was set).
//
// If writing to the output file fails and the original error is nil, returns
// the write error. If the original error is not nil, just logs the write error
// and returns the original error.
func (c *Subcommand) writeJSONOutput(result interface{}, err error) error {
	if c.jsonOutput == "" {
		return err
	}

	// We don't want to create the file if we can't serialize. So serialize first.
	var body struct {
		Error  string      `json:"error,omitempty"`
		Result interface{} `json:"result,omitempty"`
	}
	if err != nil {
		body.Error = err.Error()
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

	if e = ioutil.WriteFile(c.jsonOutput, out, 0666); e != nil {
		if err == nil {
			err = e
		} else {
			fmt.Fprintf(os.Stderr, "Failed write JSON output to %s: %s\n", c.jsonOutput, e)
		}
	}
	return err
}
