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

package cli

import (
	"context"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging/gologger"
)

type Params struct {
	DefaultBuildbucketHost string
	Auth                   auth.Options
}

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
}

// application creates the application and configures its subcommands.
// Ignores p.Auth.Scopes.
func application(p Params) *cli.Application {
	p.Auth.Scopes = []string{
		auth.OAuthScopeEmail,
		gerrit.OAuthScope,
	}

	return &cli.Application{
		Name:  "bb",
		Title: "A CLI client for buildbucket.",
		Context: func(ctx context.Context) context.Context {
			return logCfg.Use(ctx)
		},
		Commands: []*subcommands.Command{
			cmdAdd(p),
			cmdGet(p),
			cmdLS(p),
			cmdLog(p),
			cmdCancel(p),
			cmdBatch(p),
			cmdCollect(p),

			{},
			authcli.SubcommandLogin(p.Auth, "auth-login", false),
			authcli.SubcommandLogout(p.Auth, "auth-logout", false),
			authcli.SubcommandInfo(p.Auth, "auth-info", false),

			{},
			subcommands.CmdHelp,
		},
	}
}

func Main(p Params, args []string) int {
	return subcommands.Run(application(p), fixflagpos.FixSubcommands(args))
}
