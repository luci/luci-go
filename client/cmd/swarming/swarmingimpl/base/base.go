// Copyright 2023 The LUCI Authors.
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

// Package base is shared functionality used by all Swarming CLI subcommands.
//
// Its things like registering command line flags, setting up the context,
// writing output.
package base

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/output"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/swarming/client/swarming"
)

// Subcommand is implemented by individual Swarming subcommands.
type Subcommand interface {
	// RegisterFlags registers subcommand flags, if any.
	RegisterFlags(fs *flag.FlagSet)
	// ParseInputs extracts information from flags, CLI args and environ.
	ParseInputs(ctx context.Context, args []string, env subcommands.Env, extra Extra) error
	// Execute executes the subcommand.
	Execute(ctx context.Context, svc swarming.Client, sink *output.Sink, extra Extra) error
}

// Extra is passed to an executing subcommand and it contains any additional
// context the subcommand may need.
type Extra struct {
	// AuthFlags is parsed auth flags.
	//
	// Can be used to construct any extra authenticated clients (like CAS client).
	AuthFlags AuthFlags

	// ServerURL is parsed and validated URL of the swarming host.
	//
	// It has its path stripped.
	ServerURL *url.URL

	// OutputJSON is path to the file where the results will be stored, if any.
	//
	// It is empty if JSON results aren't being written anywhere. It can be
	// literal `-` if results are written to stdout.
	//
	// No need to manually write anything to it. Instead write to `sink` passed
	// to Execute. This value is exposed to allow to reference it in logs.
	OutputJSON string

	// Standard output stream, perhaps redirected somewhere.
	Stdout io.Writer

	// Standard error stream, perhaps redirected somewhere.
	Stderr io.Writer
}

// Unlimited can be passed as Features.MaxArgs to indicate no limit.
const Unlimited = -1

// Features customize "standard" behaviors exposed by a subcommand.
//
// If a multiple subcommands use a particular behavior, it is represented by
// a feature here.
type Features struct {
	// MinArgs is the minimum number of expected positional arguments.
	MinArgs int
	// MaxArgs is the maximum number of expected positional arguments.
	MaxArgs int
	// MeasureDuration indicates to measure and log how long the command took.
	MeasureDuration bool
	// UsesCAS indicates if `-cas-addr` flag should be exposed.
	UsesCAS bool
	// OutputJSON indicates if the command supports emitting JSON output.
	OutputJSON OutputJSON
}

// OutputJSON describes behavior of the command line flag with JSON file path.
type OutputJSON struct {
	// Enabled indicates if JSON output flag (`-json-output`) is enabled.
	Enabled bool
	// DeprecatedAliasFlag is how to name a flag that aliases `-json-output'.
	DeprecatedAliasFlag string
	// Usage is a flag usage string, if the command needs a custom one.
	Usage string
	// DefaultToStdout if true indicate to write to stdout if the flag is unset.
	DefaultToStdout bool
}

// AuthFlags is registered in a flag set and creates http.Client and CAS Client.
//
// It encapsulates a way of getting credentials and constructing RPC transports.
// It is constructed by whoever assembles the final cli.Application.
type AuthFlags interface {
	// Register registers auth flags to the given flag set.
	Register(f *flag.FlagSet)
	// Parse parses auth flags.
	Parse() error
	// NewHTTPClient creates an authenticating http.Client.
	NewHTTPClient(ctx context.Context) (*http.Client, error)
	// NewRBEClient creates an authenticating RBE Client.
	NewRBEClient(ctx context.Context, addr string, instance string) (*rbeclient.Client, error)
}

// NewCommandRun creates a CommandRun that runs the given subcommand.
func NewCommandRun(authFlags AuthFlags, impl Subcommand, feats Features) *CommandRun {
	cr := &CommandRun{
		authFlags: authFlags,
		impl:      impl,
		feats:     feats,
	}

	// Register all common flags.
	cr.authFlags.Register(&cr.Flags)
	cr.Flags.BoolVar(&cr.quiet, "quiet", false, "Log at Warning verbosity level.")
	cr.Flags.BoolVar(&cr.verbose, "verbose", false, "Log at Debug verbosity level.")
	cr.Flags.StringVar(&cr.rawServerURL, "server", "", fmt.Sprintf("URL or a hostname of a swarming server to call. If not set defaults to $%s. Required.", swarming.ServerEnvVar))
	cr.Flags.StringVar(&cr.rawServerURL, "S", "", "Alias for -server.")
	if feats.UsesCAS {
		cr.Flags.StringVar(&cr.casAddr, "cas-addr", casclient.AddrProd, "CAS service address.")
	}

	// Register the JSON output flag(s).
	if feats.OutputJSON.Enabled {
		usage := "A path to write operation results to as JSON. If literal \"-\", then stdout."
		if feats.OutputJSON.Usage != "" {
			usage = feats.OutputJSON.Usage
		}
		defaultVal := ""
		if feats.OutputJSON.DefaultToStdout {
			defaultVal = "-"
		}
		cr.Flags.StringVar(&cr.jsonOutput, "json-output", defaultVal, usage)
		if feats.OutputJSON.DeprecatedAliasFlag != "" {
			cr.Flags.StringVar(&cr.jsonOutput, feats.OutputJSON.DeprecatedAliasFlag, defaultVal, "Alias for -json-output for compatibility with older callers. Use -json-output instead.")
		}
	}

	// Register custom flags exposed by the subcommand.
	impl.RegisterFlags(&cr.Flags)

	return cr
}

// CommandRun implements the command part of subcommand processing.
//
// It is responsible for registering and parsing flags, setting up the root
// context and calling the subcommand implementation.
type CommandRun struct {
	subcommands.CommandRunBase

	// Flags.
	quiet        bool      // -quite
	verbose      bool      // -verbose
	authFlags    AuthFlags // e.g. -service-account-json, depends on implementation
	rawServerURL string    // -server
	casAddr      string    // -cas-addr if UsesCAS is true
	jsonOutput   string    // -json-output if feats.OutputJSON is enabled

	// Not flags.
	serverURL *url.URL   // parsed -server
	impl      Subcommand // whatever was passed to NewCommandRun
	feats     Features   // whatever was passed to NewCommandRun

	// Testing helpers.
	testingContext  context.Context
	testingSwarming swarming.Client
	testingStderr   io.Writer
	testingStdout   io.Writer
	testingEnv      subcommands.Env
	testingErr      *error
}

// TestingMocks is used in tests to mock dependencies.
func (cr *CommandRun) TestingMocks(ctx context.Context, svc swarming.Client, env subcommands.Env, err *error, stdout, stderr io.Writer) {
	cr.testingContext = ctx
	cr.testingSwarming = svc
	cr.testingStdout = stdout
	cr.testingStderr = stderr
	cr.testingEnv = env
	cr.testingErr = err
}

// stdout is stdout stream to use, perhaps mocked in tests.
func (cr *CommandRun) stdout() io.Writer {
	if cr.testingStdout != nil {
		return cr.testingStdout
	}
	return os.Stdout
}

// stderr is stderr stream to use, perhaps mocked in tests.
func (cr *CommandRun) stderr() io.Writer {
	if cr.testingStderr != nil {
		return cr.testingStderr
	}
	return os.Stderr
}

// Run is part of subcommands.CommandRun interface.
func (cr *CommandRun) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	// Validate was given expected number of positional arguments.
	var msg string
	switch {
	case cr.feats.MaxArgs == 0 && len(args) != 0:
		msg = fmt.Sprintf("unexpected arguments: %v\n", args)
	case cr.feats.MinArgs > 0 && len(args) < cr.feats.MinArgs:
		if cr.feats.MaxArgs == cr.feats.MinArgs {
			msg = fmt.Sprintf("expecting exactly %d argument(s), but got %d", cr.feats.MinArgs, len(args))
		} else {
			msg = fmt.Sprintf("expecting at least %d argument(s), but got %d", cr.feats.MinArgs, len(args))
		}
	case cr.feats.MaxArgs != Unlimited && len(args) > cr.feats.MaxArgs:
		if cr.feats.MaxArgs == cr.feats.MinArgs {
			msg = fmt.Sprintf("expecting exactly %d argument(s), but got %d", cr.feats.MinArgs, len(args))
		} else {
			msg = fmt.Sprintf("expecting at most %d argument(s), but got %d", cr.feats.MaxArgs, len(args))
		}
	}
	if msg != "" {
		fmt.Fprintf(cr.stderr(), "%s: %s\n", app.GetName(), msg)
		return 1
	}

	// Parse flags, positional arguments and environment variables.
	if err := cr.parseCommonFlags(env); err != nil {
		fmt.Fprintf(cr.stderr(), "%s: %s\n", app.GetName(), err)
		return 1
	}

	// Prepare the base context with configured logging.
	ctx := cr.testingContext
	if ctx == nil {
		ctx = cli.GetContext(app, cr, env)
	}
	var level logging.Level
	switch {
	case cr.quiet && !cr.verbose:
		level = logging.Warning
	case cr.verbose:
		level = logging.Debug
	default:
		level = logging.Info
	}
	ctx = logging.SetLevel(ctx, level)

	// Terminate everything on Ctrl+C.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer signals.HandleInterrupt(func() {
		logging.Warningf(ctx, "Canceled via Ctrl+C or SIGTERM!")
		cancel()
	})()

	if err := cr.impl.ParseInputs(ctx, args, env, Extra{
		AuthFlags:  cr.authFlags,
		ServerURL:  cr.serverURL,
		OutputJSON: cr.jsonOutput,
		Stdout:     cr.stdout(),
		Stderr:     cr.stderr(),
	}); err != nil {
		fmt.Fprintf(cr.stderr(), "%s: %s\n", app.GetName(), err)
		return 1
	}

	// Execute the subcommand and store the output.
	if err := cr.execute(ctx); err != nil {
		errors.Log(ctx, err)
		if cr.testingErr != nil {
			*cr.testingErr = err
		}
		return 1
	}

	return 0
}

// parseCommonFlags parses the common flags.
func (cr *CommandRun) parseCommonFlags(env subcommands.Env) error {
	if err := cr.authFlags.Parse(); err != nil {
		return err
	}

	// Parse and validate Swarming host URL.
	if cr.rawServerURL == "" {
		cr.rawServerURL = env[swarming.ServerEnvVar].Value
	}
	if cr.rawServerURL == "" {
		return errors.Reason("must provide -server or set $%s env var", swarming.ServerEnvVar).Err()
	}
	var err error
	if cr.serverURL, err = lhttp.ParseHostURL(cr.rawServerURL); err != nil {
		return errors.Annotate(err, "invalid -server %q", cr.rawServerURL).Err()
	}

	return nil
}

// exec executes the subcommand and stores the JSON output.
func (cr *CommandRun) execute(ctx context.Context) error {
	// Figure out where to stream JSON output.
	var sink *output.Sink
	var closeOutput func() error
	switch cr.jsonOutput {
	case "":
		// Don't write JSON output at all.
		sink = output.NewDiscardingSink()
		closeOutput = func() error { return nil }
	case "-":
		// Write JSON output to stdout, but do not close it.
		sink = output.NewSink(cr.stdout())
		closeOutput = func() error { return nil }
	default:
		// Write JSON output to a file and close it.
		jsonFile, err := os.Create(cr.jsonOutput)
		if err != nil {
			return errors.Annotate(err, "opening JSON output file for writing").Err()
		}
		sink = output.NewSink(jsonFile)
		closeOutput = func() error { return jsonFile.Close() }
	}

	svc := cr.testingSwarming
	if svc == nil {
		cl, err := cr.authFlags.NewHTTPClient(ctx)
		if err != nil {
			return err
		}
		svc, err = swarming.NewClient(ctx, swarming.ClientOptions{
			ServiceURL:          cr.serverURL.String(),
			RBEAddr:             cr.casAddr,
			AuthenticatedClient: cl,
			RBEClientFactory:    cr.authFlags.NewRBEClient,
		})
		if err != nil {
			return err
		}
	}

	started := clock.Now(ctx)

	err := cr.impl.Execute(ctx, svc, sink, Extra{
		AuthFlags:  cr.authFlags,
		ServerURL:  cr.serverURL,
		OutputJSON: cr.jsonOutput,
		Stdout:     cr.stdout(),
		Stderr:     cr.stderr(),
	})
	svc.Close(ctx)

	// Close JSON output and figure out the final error.
	closeErr := sink.Finalize()
	if closeErr == nil {
		closeErr = closeOutput()
	} else {
		_ = closeOutput() // prefer Finalize error as the main error
	}
	if closeErr != nil {
		logging.Errorf(ctx, "Failed to finalize JSON output: %s", closeErr)
		if err == nil {
			err = errors.Annotate(closeErr, "finalizing JSON output").Err()
		}
	}

	if cr.feats.MeasureDuration {
		dt := clock.Since(ctx, started)
		if err == nil {
			logging.Infof(ctx, "The command completed in %s", dt.Round(time.Millisecond))
		} else {
			logging.Infof(ctx, "The command failed in %s", dt.Round(time.Millisecond))
		}
	}

	return err
}
