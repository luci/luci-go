// Copyright 2026 The LUCI Authors.
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

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging/gologger"
)

// Params is the parameters for the Turbo CI CLI client.
type Params struct {
	Auth               auth.Options
	DefaultTurboCIHost string
}

// application creates the application and configures its subcommands.
// Ignores p.Auth.Scopes.
func application(p Params) *cli.Application {
	return &cli.Application{
		Name:  "turboci",
		Title: "A CLI client for TurboCI.",
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},
		Commands: []*subcommands.Command{
			// TODO(b/502646298): Optimization - add a subcommand to keep a
			// live connection throughout a build and use it for all TurboCI
			// calls.
			cmdReadWorkplan(p),
			cmdQueryNodes(p),
			cmdWriteNodes(p),

			{}, // a separator
			authcli.SubcommandLogin(p.Auth, "auth-login", false),
			authcli.SubcommandLogout(p.Auth, "auth-logout", false),
			authcli.SubcommandInfo(p.Auth, "auth-info", false),

			{}, // a separator
			subcommands.CmdHelp,
			versioncli.CmdVersion(userAgent),
		},
	}
}

func Main(p Params, args []string) int {
	return subcommands.Run(application(p), fixflagpos.FixSubcommands(args))
}
