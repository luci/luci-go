// Copyright 2016 The LUCI Authors.
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

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/grpc/prpc"
)

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
}

// exit codes:
const (
	ecInvalidCommandLine = -iota
	ecAuthenticatedClientError
	ecOtherError
)

type exitCode struct {
	err  error
	code int
}

func (e *exitCode) Error() string { return e.err.Error() }

// cmdRun is a base of all prpc subcommands.
// It defines some common flags, such as logging and auth, and useful methods.
type cmdRun struct {
	subcommands.CommandRunBase
	verbose       bool
	forceInsecure bool
	auth          authcli.Flags
}

// ModifyContext implements cli.ContextModificator.
func (r *cmdRun) ModifyContext(ctx context.Context) context.Context {
	if r.verbose {
		ctx = logging.SetLevel(ctx, logging.Debug)
	}
	return ctx
}

// registerBaseFlags registers common flags used by all subcommands.
func (r *cmdRun) registerBaseFlags(defaultAuthOpts auth.Options) {
	r.Flags.BoolVar(&r.verbose, "verbose", false, "Enable more logging.")
	r.Flags.BoolVar(&r.forceInsecure, "force-insecure", false, "Force HTTP instead of HTTPS")
	r.auth.Register(&r.Flags, defaultAuthOpts)
	r.auth.RegisterIDTokenFlags(&r.Flags)
}

func (r *cmdRun) authenticatedClient(ctx context.Context, host string) (*prpc.Client, error) {
	authOpts, err := r.auth.Options()
	if err != nil {
		return nil, err
	}
	if authOpts.UseIDTokens && authOpts.Audience == "" {
		authOpts.Audience = "https://" + host
	}
	creds, err := auth.NewAuthenticator(ctx, auth.OptionalLogin, authOpts).PerRPCCredentials()
	if err != nil {
		return nil, err
	}
	client := prpc.Client{
		Host:    host,
		Options: prpc.DefaultOptions(),
	}
	client.Options.Insecure = r.forceInsecure || lhttp.IsLocalHost(host)
	client.Options.PerRPCCredentials = creds
	return &client, nil
}

// argErr prints an err and usage to stderr and returns an exit code.
func (r *cmdRun) argErr(shortDesc, usageLine, format string, a ...any) int {
	if format != "" {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
	fmt.Fprintln(os.Stderr, shortDesc)
	fmt.Fprintln(os.Stderr, usageLine)
	fmt.Fprintln(os.Stderr, "\nFlags:")
	r.Flags.PrintDefaults()
	return ecInvalidCommandLine
}

// done prints err to stderr if it is not nil and returns an exit code.
func (r *cmdRun) done(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if err, ok := err.(*exitCode); ok {
			return err.code
		}
		return ecOtherError
	}
	return 0
}

func getApplication(defaultAuthOpts auth.Options) *cli.Application {
	return &cli.Application{
		Name:  "prpc",
		Title: "Provisional Remote Procedure Call CLI",
		Context: func(ctx context.Context) context.Context {
			return logCfg.Use(ctx)
		},
		Commands: []*subcommands.Command{
			cmdCall(defaultAuthOpts),
			cmdShow(defaultAuthOpts),

			{ /* spacer */ },

			authcli.SubcommandLogin(defaultAuthOpts, "login", false),
			authcli.SubcommandLogout(defaultAuthOpts, "logout", false),

			{ /* spacer */ },

			subcommands.CmdHelp,
		},
	}
}

func main() {
	app := getApplication(chromeinfra.DefaultAuthOptions())
	os.Exit(subcommands.Run(app, os.Args[1:]))
}
