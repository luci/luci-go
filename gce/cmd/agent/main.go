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

// Package main contains an agent for connecting to a Swarming server.
package main

import (
	"context"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// cmdRunBase is the base struct all subcommands should embed.
// Implements cli.ContextModificator.
type cmdRunBase struct {
	subcommands.CommandRunBase
	authFlags authcli.Flags
}

// Initialize registers common flags.
func (b *cmdRunBase) Initialize() {
	opts := chromeinfra.DefaultAuthOptions()
	b.authFlags.Register(b.GetFlags(), opts)
}

// ModifyContext returns a new context with configured logging and a *SwarmingClient installed.
// Implements cli.ContextModificator.
func (b *cmdRunBase) ModifyContext(c context.Context) context.Context {
	c = logging.SetLevel(gologger.StdConfig.Use(c), logging.Debug)
	opts, err := b.authFlags.Options()
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		panic("failed to get auth options")
	}
	http, err := auth.NewAuthenticator(c, auth.OptionalLogin, opts).Client()
	if err != nil {
		logging.Errorf(c, "%s", err.Error())
		panic("failed to get authenticator")
	}
	return withClient(c, &SwarmingClient{
		Client:           http,
		PlatformStrategy: newStrategy(),
	})
}

// New returns a new agent application.
func New() *cli.Application {
	return &cli.Application{
		Name:  "agent",
		Title: "GCE agent",
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			newConnectCmd(),
		},
	}
}

func main() {
	os.Exit(subcommands.Run(New(), os.Args[1:]))
}
