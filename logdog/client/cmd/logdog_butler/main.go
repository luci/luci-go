// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/clock/clockflag"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/flag/multiflag"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/runtime/paniccatcher"
	"github.com/luci/luci-go/common/runtime/profiling"
	grpcLogging "github.com/luci/luci-go/grpc/logging"
	"github.com/luci/luci-go/logdog/client/butler"
	"github.com/luci/luci-go/logdog/client/butler/output"
	"github.com/luci/luci-go/logdog/client/butlerlib/streamproto"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"

	"github.com/luci/luci-go/hardcoded/chromeinfra"
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

// application is the Butler application instance and its runtime configuration
// and state.
type application struct {
	cli.Application
	context.Context

	project             cfgtypes.ProjectName
	prefix              types.StreamName
	coordinatorHost     string
	outputWorkers       int
	outputConfig        outputConfigFlag
	ioKeepAliveInterval clockflag.Duration

	authFlags authcli.Flags

	globalTags   streamproto.TagMap
	maxBufferAge clockflag.Duration
	noBufferLogs bool

	prof profiling.Profiler

	client *http.Client

	// ncCtx is a context that will not be cancelled when cancelFunc is called.
	ncCtx      context.Context
	cancelFunc func()
}

func (a *application) addFlags(fs *flag.FlagSet) {
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

	fs.Var(&a.project, "project",
		"The log prefix's project name (required).")
	fs.Var(&a.prefix, "prefix",
		"Prefix to apply to all stream names.")
	fs.StringVar(&a.coordinatorHost, "coordinator-host", "",
		"The Coordinator host to pass on to subprocesses.")
	fs.Var(&a.outputConfig, "output",
		"The output name and configuration. Specify 'help' for more information.")
	fs.IntVar(&a.outputWorkers, "output-workers", butler.DefaultOutputWorkers,
		"The maximum number of parallel output dispatches.")
	fs.Var(&a.globalTags, "tag",
		"Specify key[=value] tags to be applied to all log streams. Individual treams may override. Can "+
			"be specified multiple times.")
	fs.Var(&a.maxBufferAge, "output-max-buffer-age",
		"Send buffered messages if they've been held for longer than this period.")
	fs.BoolVar(&a.noBufferLogs, "output-no-buffer", false,
		"If true, dispatch logs immediately. Setting this flag simplifies output at the expense "+
			"of wire-format efficiency.")
	fs.Var(&a.ioKeepAliveInterval, "io-keepalive-stderr",
		"If supplied, periodically write messages to STDERR if data is received on any Butler stream.")
}

func (a *application) authenticator(ctx context.Context) (*auth.Authenticator, error) {
	opts, err := a.authFlags.Options()
	if err != nil {
		return nil, err
	}
	return auth.NewAuthenticator(ctx, auth.SilentLogin, opts), nil
}

// getOutputFactory returns the currently-configured output factory.
func (a *application) getOutputFactory() (outputFactory, error) {
	factory := a.outputConfig.getFactory()
	if factory == nil {
		return nil, errors.New("main: No output is configured")
	}
	return factory, nil
}

// runWithButler is an execution harness that adds application-level management
// to a Butler run.
func (a *application) runWithButler(out output.Output, runFunc func(*butler.Butler) error) error {

	// Start our Profiler.
	a.prof.Logger = log.Get(a)
	if err := a.prof.Start(); err != nil {
		return fmt.Errorf("failed to start Profiler: %v", err)
	}
	defer a.prof.Stop()

	// Instantiate our Butler.
	butlerOpts := butler.Config{
		Project:             a.project,
		Prefix:              a.prefix,
		GlobalTags:          a.globalTags,
		MaxBufferAge:        time.Duration(a.maxBufferAge),
		BufferLogs:          !a.noBufferLogs,
		Output:              out,
		OutputWorkers:       a.outputWorkers,
		TeeStdout:           os.Stdout,
		TeeStderr:           os.Stderr,
		IOKeepAliveInterval: time.Duration(a.ioKeepAliveInterval),
		IOKeepAliveWriter:   os.Stderr,
	}
	b, err := butler.New(a, butlerOpts)
	if err != nil {
		return err
	}

	// Log the Butler's emitted streams.
	defer func() {
		if r := out.Record(); r != nil {
			// Log detail stream record.
			streams := make([]string, 0, len(r.Streams))
			for k := range r.Streams {
				streams = append(streams, string(k))
			}
			sort.Strings(streams)

			for i, stream := range streams {
				rec := r.Streams[types.StreamPath(stream)]

				ranges := make([]string, len(rec.Ranges))
				for i, rng := range rec.Ranges {
					ranges[i] = rng.String()
				}
				log.Infof(a, "%d) Stream [%s]: %s", i, stream, strings.Join(ranges, " "))
			}
		} else {
			// No record; display stream overview.
			s := b.Streams()
			paths := make([]types.StreamPath, len(s))
			for i, sn := range s {
				paths[i] = a.prefix.Join(sn)
			}
			log.Fields{
				"count":   len(paths),
				"streams": paths,
			}.Infof(a, "Butler emitted %d stream(s).", len(paths))
		}
	}()

	// Execute our Butler run function with the instantiated Butler.
	if err := runFunc(b); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(a, "Butler terminated with error.")
		a.cancelFunc()
		return err
	}

	return nil
}

// logAnnotatedErr logs the full stack trace from an annotated error to the
// installed logger at error level.
func logAnnotatedErr(ctx context.Context, err error, f string, args ...interface{}) {
	if err == nil {
		return
	}

	var buf bytes.Buffer
	st := errors.RenderStack(err)
	if _, derr := st.DumpTo(&buf); derr != nil {
		// This can't really fail, since we're rendering to a Buffer.
		panic(derr)
	}

	nargs := make([]interface{}, len(args)+1)
	nargs[copy(nargs, args)] = buf.Bytes()

	if f == "" {
		f = "Captured error stack:"
	}
	log.Errorf(ctx, f+"\n%s", nargs...)
}

func mainImpl(ctx context.Context, defaultAuthOpts auth.Options, argv []string) int {
	defaultAuthOpts.Scopes = allOutputScopes()

	a := &application{
		Context: ctx,
		Application: cli.Application{
			Name:    "butler",
			Title:   "Log collection and streaming bootstrap.",
			Context: func(context.Context) context.Context { return ctx },
			Commands: []*subcommands.Command{
				subcommands.CmdHelp,
				subcommandRun,
				subcommandStream,
				subcommandServe,

				authcli.SubcommandLogin(defaultAuthOpts, "auth-login", false),
				authcli.SubcommandLogout(defaultAuthOpts, "auth-logout", false),
				authcli.SubcommandInfo(defaultAuthOpts, "auth-info", false),
			},
		},
	}
	// Install logging configuration flags.
	flags := flag.NewFlagSet("flags", flag.ExitOnError)
	logConfig := log.Config{
		Level: log.Warning,
	}
	logConfig.AddFlags(flags)
	a.addFlags(flags)
	a.authFlags.Register(flags, defaultAuthOpts)
	a.prof.AddFlags(flags)

	// Parse the top-level flag set.
	if err := flags.Parse(argv); err != nil {
		log.WithError(err).Errorf(a, "Failed to parse command-line.")
		return configErrorReturnCode
	}

	a.Context = logConfig.Set(a.Context)

	// Install a global gRPC logger adapter. This routes gRPC log messages that
	// are emitted through our logger. We only log gRPC prints if our logger is
	// configured to log info-level or lower.
	grpcLogging.Install(log.Get(a.Context), log.IsLogging(a.Context, log.Info))

	if err := a.project.Validate(); err != nil {
		log.WithError(err).Errorf(a, "Invalid project (-project).")
		return configErrorReturnCode
	}

	// Validate our Prefix; generate a user prefix if one was not supplied.
	prefix := a.prefix
	if prefix == "" {
		// Auto-generate a prefix.
		prefix, err := generateRandomUserPrefix(a)
		if err != nil {
			log.WithError(err).Errorf(a, "Failed to generate user prefix.")
			return configErrorReturnCode
		}
		a.prefix = prefix
	}

	// Signal handler to catch 'Control-C'. This will gracefully shutdown the
	// butler the first time a signal is received. It will die abruptly if the
	// signal continues to be received.
	//
	// The specific signals used here are OS-specific.
	a.ncCtx = a.Context
	a.Context, a.cancelFunc = context.WithCancel(a.Context)
	signalC := make(chan os.Signal, 1)
	signal.Notify(signalC, interruptSignals...)
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
		"prefix": a.prefix,
	}.Infof(a, "Using session prefix.")
	if err := a.prefix.Validate(); err != nil {
		log.WithError(err).Errorf(a, "Invalid session prefix.")
		return configErrorReturnCode
	}

	// Run our subcommand (and parse subcommand flags).
	return subcommands.Run(a, flags.Args())
}

// Main execution function. This immediately jumps to 'mainImpl' and uses its
// result as an exit code.
func main() {
	mathrand.SeedRandomly()

	ctx := context.Background()
	ctx = gologger.StdConfig.Use(ctx)

	// Exit with the specified return code.
	rc := 0
	defer func() {
		log.Infof(log.SetField(ctx, "returnCode", rc), "Terminating.")
		os.Exit(rc)
	}()

	paniccatcher.Do(func() {
		rc = mainImpl(ctx, chromeinfra.DefaultAuthOptions(), os.Args[1:])
	}, func(p *paniccatcher.Panic) {
		log.Fields{
			"panic.error": p.Reason,
		}.Errorf(ctx, "Panic caught in main:\n%s", p.Stack)
		rc = runtimeErrorReturnCode
	})
}
