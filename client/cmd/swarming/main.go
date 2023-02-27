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
//
// The reference server python implementation documentation can be found at
// https://github.com/luci/luci-py/tree/master/appengine/swarming/doc
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/hardcoded/chromeinfra"
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
		return nil, errors.Reason("AuthFlags.Parse() must be called").Err()
	}
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, *af.parsedOpts).Client()
}

func (af *authFlags) NewRBEClient(ctx context.Context, addr string, instance string) (*client.Client, error) {
	if af.parsedOpts == nil {
		return nil, errors.Reason("AuthFlags.Parse() must be called").Err()
	}
	return casclient.NewLegacy(ctx, addr, instance, *af.parsedOpts, true)
}

func getApplication() *subcommands.DefaultApplication {
	authOpts := chromeinfra.DefaultAuthOptions()
	af := &authFlags{defaultOpts: authOpts}

	return &subcommands.DefaultApplication{
		Name:  "swarming",
		Title: "Client tool to access a swarming server.",
		// Keep in alphabetical order of their name.
		Commands: []*subcommands.Command{
			subcommands.Section("task related commands\n"),
			swarmingimpl.CmdCancelTask(af),
			swarmingimpl.CmdCollect(af),
			swarmingimpl.CmdReproduce(af),
			swarmingimpl.CmdRequestShow(af),
			swarmingimpl.CmdSpawnTasks(af),
			swarmingimpl.CmdTasks(af),
			swarmingimpl.CmdTrigger(af),
			subcommands.Section("bot related commands\n"),
			swarmingimpl.CmdBots(af),
			swarmingimpl.CmdDeleteBots(af),
			swarmingimpl.CmdTerminateBot(af),
			subcommands.Section("other commands\n"),
			subcommands.CmdHelp,
			authcli.SubcommandInfo(authOpts, "whoami", false),
			authcli.SubcommandLogin(authOpts, "login", false),
			authcli.SubcommandLogout(authOpts, "logout", false),
			versioncli.CmdVersion(swarmingimpl.SwarmingVersion),
		},

		EnvVars: map[string]subcommands.EnvVarDefinition{
			swarmingimpl.TaskIDEnvVar: {
				Advanced: true,
				ShortDesc: ("Used when processing new triggered tasks. Is used as the " +
					"parent task ID for the newly triggered tasks."),
			},
		},
	}
}

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	os.Exit(subcommands.Run(getApplication(), nil))
}
