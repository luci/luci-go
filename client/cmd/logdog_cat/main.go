// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logdog/coordinator"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/prpc"
)

func init() {
	prpc.DefaultUserAgent = "logdog_cat"
}

type application struct {
	cli.Application
	context.Context

	project     config.ProjectName
	authFlags   authcli.Flags
	coordinator string
	insecure    bool

	coord *coordinator.Client
}

func (a *application) addToFlagSet(ctx context.Context, fs *flag.FlagSet) {
	fs.StringVar(&a.coordinator, "host", "",
		"The LogDog Coordinator [host][:port].")
	fs.BoolVar(&a.insecure, "insecure", false,
		"Use insecure transport for RPC.")
	fs.Var(&a.project, "project",
		"The log stream's project.")
}

func (a *application) validate() error {
	if a.coordinator == "" {
		return errors.New("main: missing coordinator URL (-url)")
	}
	// TODO(dnj): Error on empty project once that's disallowed.
	if a.project != "" {
		if err := a.project.Validate(); err != nil {
			return fmt.Errorf("main: invalid project name (-project): %s", err)
		}
	}
	return nil
}

// splitPath converts between a possible user-facing "unified" stream path
// (e.g., "project/path...") to separate project/path values.
//
// If a project is supplied via command-line, the path is returned directly
// along with the project. If no project is supplied, the first slash-delimited
// component of "p" is used as the project name.
func (a *application) splitPath(p string) (config.ProjectName, string, bool, error) {
	if a.project != "" {
		return a.project, p, false, nil
	}

	parts := strings.SplitN(p, types.StreamNameSepStr, 2)

	project := config.ProjectName(parts[0])
	if err := project.Validate(); err != nil {
		return "", "", false, fmt.Errorf("invalid project name %q: %v", project, err)
	}

	if len(parts) == 2 {
		p = parts[1]
	}
	return project, p, true, nil
}

func mainImpl() int {
	ctx := context.Background()
	ctx = gologger.StdConfig.Use(ctx)

	authOptions := auth.Options{
		Scopes: coordinator.Scopes,
	}

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
				authcli.SubcommandLogin(authOptions, "auth-login"),
				authcli.SubcommandLogout(authOptions, "auth-logout"),
				authcli.SubcommandInfo(authOptions, "auth-info"),
			},
		},
	}
	loggingConfig := log.Config{
		Level: log.Level(log.Info),
	}

	flags := &flag.FlagSet{}
	a.addToFlagSet(ctx, flags)
	loggingConfig.AddFlags(flags)
	a.authFlags.Register(flags, authOptions)

	// Parse flags.
	if err := flags.Parse(os.Args[1:]); err != nil {
		log.Errorf(log.SetError(ctx, err), "Failed to parse command-line.")
		return 1
	}

	// Install our log formatter.
	ctx = loggingConfig.Set(ctx)

	if err := a.validate(); err != nil {
		log.Errorf(log.SetError(ctx, err), "Invalid application configuration.")
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
		Host:    a.coordinator,
		Options: prpc.DefaultOptions(),
	}
	prpcClient.Options.Insecure = a.insecure

	a.coord = coordinator.NewClient(prpcClient)
	a.Context = ctx
	return subcommands.Run(&a, flags.Args())
}

func main() {
	os.Exit(mainImpl())
}
