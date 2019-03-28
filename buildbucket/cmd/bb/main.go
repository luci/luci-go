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
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/hardcoded/chromeinfra"
)

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
}

// GetApplication creates the application and configures its subcommands.
func GetApplication(defaultAuthOpts auth.Options) *cli.Application {
	return &cli.Application{
		Name:  "buildbucket",
		Title: "A CLI client for buildbucket.",
		Context: func(ctx context.Context) context.Context {
			return logCfg.Use(ctx)
		},
		Commands: []*subcommands.Command{
			cmdAdd(defaultAuthOpts),
			cmdGet(defaultAuthOpts),
			cmdCancel(defaultAuthOpts),
			cmdBatch(defaultAuthOpts),
			cmdCollect(defaultAuthOpts),

			{},
			authcli.SubcommandLogin(defaultAuthOpts, "auth-login", false),
			authcli.SubcommandLogout(defaultAuthOpts, "auth-logout", false),
			authcli.SubcommandInfo(defaultAuthOpts, "auth-info", false),

			{},
			subcommands.CmdHelp,
		},
	}
}

func main() {
	mathrand.SeedRandomly()
	app := GetApplication(chromeinfra.DefaultAuthOptions())
	os.Exit(subcommands.Run(app, os.Args[1:]))
}
