// Copyright 2015 The LUCI Authors.
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

//go:build !copybara
// +build !copybara

// Package main is a client to a Swarming server.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/swarming/client/swarming"
)

type authFlags struct {
	flags       authcli.Flags
	defaultOpts auth.Options
	parsedOpts  *auth.Options
}

func (af *authFlags) Register(f *flag.FlagSet) {
	af.flags.Register(f, af.defaultOpts)
}

func (af *authFlags) Parse() error {
	opts, err := af.flags.Options()
	if err != nil {
		return err
	}
	af.parsedOpts = &opts
	return nil
}

func (af *authFlags) NewHTTPClient(ctx context.Context) (*http.Client, error) {
	if af.parsedOpts == nil {
		return nil, errors.New("AuthFlags.Parse() must be called")
	}
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, *af.parsedOpts).Client()
}

func (af *authFlags) NewRBEClient(ctx context.Context, addr string, instance string) (*client.Client, error) {
	if af.parsedOpts == nil {
		return nil, errors.New("AuthFlags.Parse() must be called")
	}
	return casclient.NewLegacy(ctx, addr, instance, *af.parsedOpts, true)
}

func getApplication() *cli.Application {
	authOpts := chromeinfra.DefaultAuthOptions()
	af := &authFlags{defaultOpts: authOpts}

	return &cli.Application{
		Name:  "swarming",
		Title: "Client tool to access a swarming server.",

		// Note: this decouples logging implementation from subcommands
		// implementation. Useful when reusing subcommands in g3.
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},

		// Keep in alphabetical order of their name.
		Commands: []*subcommands.Command{
			subcommands.Section("Tasks\n"),
			swarmingimpl.CmdCancelTask(af),
			swarmingimpl.CmdCancelTasks(af),
			swarmingimpl.CmdCollect(af),
			swarmingimpl.CmdReproduce(af),
			swarmingimpl.CmdRequestShow(af),
			swarmingimpl.CmdSpawnTasks(af),
			swarmingimpl.CmdTasks(af),
			swarmingimpl.CmdTrigger(af),
			subcommands.Section("Bots\n"),
			swarmingimpl.CmdBots(af),
			swarmingimpl.CmdDeleteBots(af),
			swarmingimpl.CmdTerminateBot(af),
			swarmingimpl.CmdBotTasks(af),
			subcommands.Section("Other\n"),
			subcommands.CmdHelp,
			authcli.SubcommandInfo(authOpts, "whoami", false),
			authcli.SubcommandLogin(authOpts, "login", false),
			authcli.SubcommandLogout(authOpts, "logout", false),
			versioncli.CmdVersion(swarming.UserAgent),
		},

		EnvVars: map[string]subcommands.EnvVarDefinition{
			swarming.ServerEnvVar: {
				ShortDesc: "URL or a hostname of a Swarming server to use by default when omitting -server or -S flag",
			},
			swarming.UserEnvVar: {
				ShortDesc: "used as \"user\" field in spawned Swarming tasks",
			},
			swarming.TaskIDEnvVar: {
				Advanced: true,
				ShortDesc: "set when running within a Swarming task to ID of that task; " +
					"used as \"parent_task_id\" field in spawned Swarming tasks",
			},
		},
	}
}

func main() {
	os.Exit(subcommands.Run(getApplication(), nil))
}
