// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
	"time"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/client/internal/flags/multiflag"
	"github.com/luci/luci-go/client/internal/logdog/butler"
	"github.com/luci/luci-go/client/internal/logdog/butler/output"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock/clockflag"
	"github.com/luci/luci-go/common/gcloud/gcps"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/paniccatcher"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
)

const (
	// flagErrorReturnCode is returned when there is an error with the Butler's
	// command-line configuration.
	configErrorReturnCode = 2

	// runtimeErrorReturnCode is returned when the execution fails due to a Butler
	// error. This is intended to help differentiate Butler errors from
	// passthrough bootstrapped subprocess errors.
	//
	// This will only be returned for runtime errors. If there is a flag error
	// or a configuration error, standard Butler return codes (likely to overlap
	// with standard process return codes) will be used.
	runtimeErrorReturnCode = 250
)

// buildScopes consumes a series of independent OAuth2 scope strings and
// combines them into a single deduplicated list.
func buildScopes(parts ...[]string) []string {
	result := []string{}
	seen := make(map[string]bool)
	for _, scopes := range parts {
		for _, s := range scopes {
			if _, ok := seen[s]; ok {
				continue
			}
			result = append(result, s)
			seen[s] = true
		}
	}
	return result
}

// butlerApplication is Butler application instance and its runtime
// configuration and state.
type butlerApplication struct {
	subcommands.DefaultApplication
	context.Context

	butler       butler.Config
	outputConfig outputConfigFlag

	authFlags authcli.Flags

	maxBufferAge clockflag.Duration
	noBufferLogs bool

	prefix     types.StreamName
	cpuProfile string

	client *http.Client

	// ncCtx is a context that will not be cancelled when cancelFunc is called.
	ncCtx      context.Context
	cancelFunc func()

	output output.Output
}

func (a *butlerApplication) addFlags(fs *flag.FlagSet) {
	a.outputConfig.Output = os.Stdout
	a.outputConfig.Description = "Select and configure message output adapter."
	a.outputConfig.Options = []multiflag.Option{
		multiflag.HelpOption(&a.outputConfig.MultiFlag),
	}

	// Add registered conditional (build tag) options.
	for _, f := range getOutputFactories() {
		a.outputConfig.AddFactory(f)
	}

	a.maxBufferAge = clockflag.Duration(butler.DefaultMaxBufferAge)

	fs.Var(&a.prefix, "prefix",
		"Prefix to apply to all stream names.")
	fs.Var(&a.outputConfig, "output",
		"The output name and configuration. Specify 'help' for more information.")
	fs.StringVar(&a.cpuProfile,
		"cpuprofile", "", "If specified, enables CPU profiling and profiles to the specified path.")
	fs.IntVar(&a.butler.OutputWorkers, "output-workers", butler.DefaultOutputWorkers,
		"The maximum number of parallel output dispatches.")
	fs.Var(&a.maxBufferAge, "output-max-buffer-age",
		"Send buffered messages if they've been held for longer than this period.")
	fs.BoolVar(&a.noBufferLogs, "output-no-buffer", false,
		"If true, dispatch logs immediately. Setting this flag simplifies output at the expense "+
			"of wire-format efficiency.")
}

func (a *butlerApplication) authenticatedClient(ctx context.Context) (*http.Client, error) {
	if a.client == nil {
		opts, err := a.authFlags.Options()
		if err != nil {
			return nil, err
		}
		opts.Context = ctx

		client, err := auth.NewAuthenticator(auth.SilentLogin, opts).Client()
		if err != nil {
			return nil, err
		}
		a.client = client
	}
	return a.client, nil
}

func (a *butlerApplication) authenticatedContext(ctx context.Context, project string) (context.Context, error) {
	if project == "" {
		return nil, errors.New("must supply a project name")
	}

	client, err := a.authenticatedClient(ctx)
	if err != nil {
		return nil, err
	}

	return cloud.WithContext(ctx, project, client), nil
}

func (a *butlerApplication) configOutput() (output.Output, error) {
	factory := a.outputConfig.getFactory()
	if factory == nil {
		return nil, errors.New("main: No output is configured")
	}

	output, err := factory.configOutput(a)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// An execution harness that adds application-level management to a Butler run.
func (a *butlerApplication) Main(runFunc func(b *butler.Butler) error) error {
	// Enable CPU profiling if specified
	if a.cpuProfile != "" {
		f, err := os.Create(a.cpuProfile)
		if err != nil {
			return fmt.Errorf("Failed to create CPU profile output: %v", err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Instantiate our Butler.
	a.butler.MaxBufferAge = time.Duration(a.maxBufferAge)
	a.butler.BufferLogs = !a.noBufferLogs
	a.butler.Output = a.output
	a.butler.TeeStdout = os.Stdout
	a.butler.TeeStderr = os.Stderr
	if err := a.butler.Validate(); err != nil {
		return err
	}

	b, err := butler.New(a, a.butler)
	if err != nil {
		return err
	}

	// Execute our Butler run function with the instantiated Butler.
	if err := runFunc(b); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(a, "Butler terminated with error.")
		a.cancelFunc()
	}
	return b.Wait()
}

func mainImpl(ctx context.Context, argv []string) int {
	authOptions := auth.Options{
		Scopes: buildScopes(
			[]string{auth.OAuthScopeEmail},
			gcps.PublisherScopes,
		),
		Context: ctx,
		Logger:  log.Get(ctx),
	}

	a := &butlerApplication{
		DefaultApplication: subcommands.DefaultApplication{
			Name:  "butler",
			Title: "Log collection and streaming bootstrap.",
			Commands: []*subcommands.Command{
				subcommands.CmdHelp,
				subcommandRun,
				subcommandStream,
				subcommandServe,

				authcli.SubcommandLogin(authOptions, "auth-login"),
				authcli.SubcommandLogout(authOptions, "auth-logout"),
				authcli.SubcommandInfo(authOptions, "auth-info"),
			},
		},
	}
	// Install logging configuration flags.
	flags := flag.NewFlagSet("flags", flag.ExitOnError)
	logConfig := log.Config{
		Level: log.Info,
	}
	logConfig.AddFlags(flags)
	a.addFlags(flags)
	a.authFlags.Register(flags, authOptions)

	// Parse the top-level flag set.
	if err := flags.Parse(argv); err != nil {
		log.Errorf(log.SetError(a, err), "Failed to parse command-line.")
		return configErrorReturnCode
	}

	ctx = logConfig.Set(ctx)

	// Validate our Prefix; generate a user prefix if one was not supplied.
	prefix := a.butler.Prefix
	if prefix == "" {
		// Auto-generate a prefix.
		prefix, err := generateRandomUserPrefix(ctx)
		if err != nil {
			log.Errorf(log.SetError(ctx, err), "Failed to generate user prefix.")
			return configErrorReturnCode
		}
		a.butler.Prefix = prefix
	}

	// Signal handler to catch 'Control-C'. This will gracefully shutdown the
	// butler the first time a signal is received. It will die abruptly if the
	// signal continues to be received.
	a.ncCtx = ctx
	ctx, a.cancelFunc = context.WithCancel(ctx)
	signalC := make(chan os.Signal, 1)
	signal.Notify(signalC, os.Interrupt)
	go func() {
		signalled := false
		for range signalC {
			if !signalled {
				signalled = true

				// First '^C'; soft-terminate
				log.Infof(a, "Flushing in response to Control-C (keyboard interrupt). Interrupt again for immediate exit.")
				a.cancelFunc()
			} else {
				// Multiple '^C'; terminate immediately
				os.Exit(1)
			}
		}
	}()
	defer func() {
		signal.Stop(signalC)
		close(signalC)
	}()

	log.Fields{
		"prefix": a.butler.Prefix,
	}.Infof(ctx, "Using session prefix.")
	if err := a.butler.Prefix.Validate(); err != nil {
		log.Errorf(log.SetError(a, err), "Invalid session prefix.")
		return configErrorReturnCode
	}

	// Configure our Butler Output.
	var err error
	a.output, err = a.configOutput()
	if err != nil {
		log.Errorf(log.SetError(ctx, err), "Failed to create output instance.")
		return runtimeErrorReturnCode
	}
	defer a.output.Close()

	// Run our subcommand (and parse subcommand flags).
	a.Context = ctx
	return subcommands.Run(a, flags.Args())
}

// Main execution function. This immediately jumps to 'mainImpl' and uses its
// result as an exit code.
func main() {
	ctx := context.Background()
	ctx = gologger.Use(ctx)

	// Exit with the specified return code.
	rc := 0
	defer func() {
		log.Infof(log.SetField(ctx, "returnCode", rc), "Terminating.")
		os.Exit(rc)
	}()

	paniccatcher.Do(func() {
		rc = mainImpl(ctx, os.Args[1:])
	}, func(p *paniccatcher.Panic) {
		log.Fields{
			"panic.error": p.Reason,
		}.Errorf(ctx, "Panic caught in main:\n%s", p.Stack)
		rc = runtimeErrorReturnCode
	})
}
