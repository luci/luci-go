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
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg"
	"go.chromium.org/luci/lucicfg/errs"
	"go.chromium.org/luci/lucicfg/internal/ui"
	"go.chromium.org/luci/lucicfg/pkg"
)

// CommandLineError is used to tag errors related to command line arguments.
//
// Subcommand.Done(..., err) will print the usage string if it finds such error.
type CommandLineError struct {
	error
}

// NewCLIError returns new CommandLineError.
func NewCLIError(msg string, args ...any) error {
	return CommandLineError{fmt.Errorf(msg, args...)}
}

// MissingFlagError is CommandLineError about a missing flag.
func MissingFlagError(flag string) error {
	return NewCLIError("%s is required", flag)
}

// Parameters can be used to customize CLI defaults.
type Parameters struct {
	AuthOptions       auth.Options // mostly for client ID and client secret
	ConfigServiceHost string       // e.g. "config.luci.app"
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

	Meta          lucicfg.Meta                 // meta config settable via CLI flags
	Vars          stringmapflag.Value          // all `-var k=v` flags
	RepoOverrides stringmapflag.Value          // all `-repo-override k=v` flags
	RepoOptions   pkg.RemoteRepoManagerOptions // `-git-debug` and `-git-concurrency` flags

	params     *Parameters    // whatever was passed to Init
	logConfig  logging.Config // for -log-level, used by ModifyContext
	authFlags  authcli.Flags  // for -service-account-json, used by ConfigService
	jsonOutput string         // for -json-output, used by Done
}

// ModifyContext implements cli.ContextModificator.
func (c *Subcommand) ModifyContext(ctx context.Context) context.Context {
	if c.RepoOptions.GitDebug {
		c.logConfig.Level = logging.Debug
	}
	return ui.WithConfig(c.logConfig.Set(ctx), ui.Config{
		Fancy: c.logConfig.Level == logging.Info,
		Term:  os.Stderr,
	})
}

// Init registers common flags.
func (c *Subcommand) Init(params Parameters) {
	c.params = &params
	c.Meta = c.DefaultMeta()

	c.logConfig.Level = logging.Info
	c.logConfig.AddFlags(&c.Flags)

	c.authFlags.Register(&c.Flags, params.AuthOptions)
	c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to.")
}

// DefaultMeta returns Meta values to use by default if not overridden via flags
// or via lucicfg.config(...).
func (c *Subcommand) DefaultMeta() lucicfg.Meta {
	if c.params == nil {
		panic("call Init first")
	}
	return lucicfg.Meta{
		ConfigServiceHost: c.params.ConfigServiceHost,
		ConfigDir:         "generated",
		// Do not enforce formatting and linting by default for now.
		LintChecks: []string{"none"},
	}
}

// AddGeneratorFlags registers flags affecting code generation.
//
// Used by subcommands that end up executing Starlark.
func (c *Subcommand) AddGeneratorFlags() {
	if c.params == nil {
		panic("call Init first")
	}
	c.Meta.AddFlags(&c.Flags)
	c.Flags.Var(&c.Vars, "var",
		"A `k=v` pair setting a value of some lucicfg.var(expose_as=...) variable, can be used multiple times (to set multiple vars).")
	c.Flags.Var(&c.RepoOverrides, "repo-override",
		"A `repo=path` pair that declares that dependencies that normally reside in the given repository should be fetched from its local checkout at the given path instead. "+
			"Example: `chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main=../luci-go`.")
	c.Flags.BoolVar(&c.RepoOptions.GitDebug, "git-debug", false, "Emit logs of git calls done to fetch dependencies, implies -log-level=debug.")
	c.Flags.IntVar(&c.RepoOptions.GitConcurrency, "git-concurrency", 0, "If positive, limits how many concurrent git fetches are allowed.")
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

// luciConfigRetryPolicy is the default grpc retry policy for LUCI Config client
const luciConfigRetryPolicy = `{
	"methodConfig": [{
		"name": [{ "service": "config.service.v2.Configs" }],
		"timeout": "120s",
		"retryPolicy": {
		  "maxAttempts": 3,
		  "initialBackoff": "0.1s",
		  "maxBackoff": "1s",
		  "backoffMultiplier": 2,
		  "retryableStatusCodes": ["UNAVAILABLE", "INTERNAL", "UNKNOWN"]
		}
	}]
}`

// Returns parsed auth.Options.
func (c *Subcommand) AuthOptions() (auth.Options, error) {
	return c.authFlags.Options()
}

// ConfigService returns a gRPC connection to the LUCI Config service.
func (c *Subcommand) ConfigService(ctx context.Context, host string) (*grpc.ClientConn, error) {
	authOpts, err := c.AuthOptions()
	if err != nil {
		return nil, err
	}
	authOpts.UseIDTokens = true
	authOpts.Audience = "https://" + host

	creds, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).PerRPCCredentials()
	if err != nil {
		return nil, errors.Fmt("failed to get credentials to access %s: %w", host, err)
	}
	conn, err := grpc.NewClient(host+":443",
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(creds),
		grpc.WithUserAgent(lucicfg.UserAgent),
		grpc.WithDefaultServiceConfig(luciConfigRetryPolicy),
	)
	if err != nil {
		return nil, errors.Fmt("cannot dial to %s: %w", host, err)
	}
	return conn, nil
}

// Done is called as the last step of processing a subcommand.
//
// It dumps the command result (or an error) to the JSON output file, prints
// the error message and generates the process exit code.
func (c *Subcommand) Done(result any, err error) int {
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
	var cmdErr CommandLineError
	if errors.As(err, &cmdErr) {
		fmt.Fprintf(os.Stderr, "Bad command line: %s.\n\n", err)
		c.Flags.Usage()
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", strings.Join(errs.Collect(err, nil), "\n"))
	}
}

// WriteJSONOutput writes result to JSON output file (if -json-output was set).
//
// If writing to the output file fails and the original error is nil, returns
// the write error. If the original error is not nil, just logs the write error
// and returns the original error.
func (c *Subcommand) writeJSONOutput(result any, err error) error {
	if c.jsonOutput == "" {
		return err
	}

	// Note: this may eventually grow to include position in the *.star source
	// code.
	type detailedError struct {
		Message string `json:"message"`
	}
	var output struct {
		Generator string          `json:"generator"`        // lucicfg version
		Error     string          `json:"error,omitempty"`  // overall error
		Errors    []detailedError `json:"errors,omitempty"` // detailed errors
		Result    any             `json:"result,omitempty"` // command-specific result
	}
	output.Generator = lucicfg.UserAgent
	output.Result = result
	if err != nil {
		output.Error = err.Error()
		for _, msg := range errs.Collect(err, nil) {
			output.Errors = append(output.Errors, detailedError{Message: msg})
		}
	}

	// We don't want to create the file if we can't serialize. So serialize first.
	// Also don't escape '<', it looks extremely ugly.
	buf := bytes.Buffer{}
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if e := enc.Encode(&output); e != nil {
		if err == nil {
			err = e
		} else {
			fmt.Fprintf(os.Stderr, "Failed to serialize JSON output: %s\n", e)
		}
		return err
	}

	if e := os.WriteFile(c.jsonOutput, buf.Bytes(), 0666); e != nil {
		if err == nil {
			err = e
		} else {
			fmt.Fprintf(os.Stderr, "Failed write JSON output to %s: %s\n", c.jsonOutput, e)
		}
	}
	return err
}
