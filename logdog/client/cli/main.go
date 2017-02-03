// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cli

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/clock/clockflag"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/grpc/prpc"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
)

func init() {
	prpc.DefaultUserAgent = "logdog CLI"
}

// Parameters is the set of application parametesr that can be supplied.
type Parameters struct {
	// Args is the command-line arguments. This should not include the command name,
	// args[0].
	Args []string

	// Host is the default host name.
	Host string

	// DefaultAuthOptions provide default values for authentication related
	// options (most notably SecretsDir: a directory with token cache).
	DefaultAuthOptions auth.Options
}

type application struct {
	cli.Application
	context.Context

	// p is the set of runtime parameters to use.
	p Parameters

	// project is the project name. This may either be a valid project name or
	// empty. Subcommands that support "unified" project-in-path paths should use
	// splitPath to get the project form the path. Those that don't should assert
	// that this is non-empty.
	project   cfgtypes.ProjectName
	authFlags authcli.Flags
	insecure  bool
	timeout   clockflag.Duration

	coord *coordinator.Client
}

func (a *application) addToFlagSet(ctx context.Context, fs *flag.FlagSet) {
	fs.StringVar(&a.p.Host, "host", a.p.Host,
		"The LogDog Coordinator [host][:port].")
	fs.BoolVar(&a.insecure, "insecure", false,
		"Use insecure transport for RPC.")
	fs.Var(&a.project, "project",
		"The log stream's project.")

	fs.Var(&a.timeout, "timeout",
		"If >0, a duration string for the maximum amount of time to wait for a log entry "+
			"before exiting with a 2. "+clockflag.DurationHelp)
}

// splitPath converts between a possible user-facing "unified" stream path
// (e.g., "project/path...") to separate project/path values.
//
// If a project is supplied via command-line, the path is returned directly
// along with the project. If no project is supplied, the first slash-delimited
// component of "p" is used as the project name.
func (a *application) splitPath(p string) (cfgtypes.ProjectName, string, bool, error) {
	if a.project != "" {
		return a.project, p, false, nil
	}

	parts := strings.SplitN(p, types.StreamNameSepStr, 2)

	project := cfgtypes.ProjectName(parts[0])
	if err := project.Validate(); err != nil {
		return "", "", false, fmt.Errorf("invalid project name %q: %v", project, err)
	}

	if len(parts) == 2 {
		p = parts[1]
	} else {
		p = ""
	}
	return project, p, true, nil
}

func (a *application) timeoutCtx(c context.Context) (context.Context, context.CancelFunc) {
	if a.timeout <= 0 {
		return context.WithCancel(c)
	}
	return context.WithTimeout(c, time.Duration(a.timeout))
}

// Main is the entry point for the CLI application.
func Main(ctx context.Context, params Parameters) int {
	ctx = gologger.StdConfig.Use(ctx)

	authOptions := params.DefaultAuthOptions
	authOptions.Scopes = coordinator.Scopes

	a := application{
		Application: cli.Application{
			Name:    "LogDog cat",
			Title:   "LogDog log data access CLI",
			Context: func(context.Context) context.Context { return ctx },

			Commands: []*subcommands.Command{
				subcommands.CmdHelp,
				newCatCommand(),
				newQueryCommand(),
				newListCommand(),
				newLatestCommand(),
				authcli.SubcommandLogin(authOptions, "auth-login", false),
				authcli.SubcommandLogout(authOptions, "auth-logout", false),
				authcli.SubcommandInfo(authOptions, "auth-info", false),
			},
		},
		p: params,
	}
	loggingConfig := log.Config{
		Level: log.Level(log.Info),
	}

	flags := &flag.FlagSet{}
	a.addToFlagSet(ctx, flags)
	loggingConfig.AddFlags(flags)
	a.authFlags.Register(flags, authOptions)

	// Parse flags.
	if err := flags.Parse(params.Args); err != nil {
		log.Errorf(log.SetError(ctx, err), "Failed to parse command-line.")
		return 1
	}

	// Install our log formatter.
	ctx = loggingConfig.Set(ctx)

	if a.p.Host == "" {
		log.Errorf(ctx, "Missing coordinator host (-host).")
		return 1
	}

	// Signal handler will cancel our context when interrupted.
	ctx, cancelFunc := context.WithCancel(ctx)
	signalC := make(chan os.Signal)
	signal.Notify(signalC, os.Interrupt, os.Kill)
	go func() {
		triggered := false
		for sig := range signalC {
			if triggered {
				os.Exit(2)
			}

			triggered = true
			log.Fields{
				"signal": sig,
			}.Warningf(ctx, "Caught signal; terminating.")
			cancelFunc()
		}
	}()
	defer func() {
		signal.Stop(signalC)
		close(signalC)
	}()

	// Instantiate our authenticated HTTP client.
	authOpts, err := a.authFlags.Options()
	if err != nil {
		log.Errorf(log.SetError(ctx, err), "Failed to create auth options.")
		return 1
	}
	httpClient, err := auth.NewAuthenticator(ctx, auth.OptionalLogin, authOpts).Client()
	if err != nil {
		log.Errorf(log.SetError(ctx, err), "Failed to create authenticated client.")
		return 1
	}

	// Get our Coordinator client instance.
	prpcClient := &prpc.Client{
		C:       httpClient,
		Host:    a.p.Host,
		Options: prpc.DefaultOptions(),
	}
	prpcClient.Options.Insecure = a.insecure

	a.coord = coordinator.NewClient(prpcClient)
	a.Context = ctx
	return subcommands.Run(&a, flags.Args())
}
