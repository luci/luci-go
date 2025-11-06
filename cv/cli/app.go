// Copyright 2021 The LUCI Authors.
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

// cli is a package implementing command line utilities for CV. These should be
// avaliable to users via the `luci-cv` binary, which can be built from
// luci/cv/cmd/luci-cv.
package cli

import (
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/flag/fixflagpos"
)

type Params struct {
	Auth auth.Options
}

func application(p Params) *cli.Application {
	p.Auth.Scopes = scopes.GerritScopeSet()
	return &cli.Application{
		Name:  "luci-cv",
		Title: "LUCI CV Command line utilities",
		Commands: []*subcommands.Command{
			cmdMatchConfig(p),

			{}, // a separator
			authcli.SubcommandLogin(p.Auth, "auth-login", false),
			authcli.SubcommandLogout(p.Auth, "auth-logout", false),
			authcli.SubcommandInfo(p.Auth, "auth-info", false),

			{}, // a separator
			subcommands.CmdHelp,
		},
	}
}

// Main is the main function of the luci-cv application.
func Main(p Params, args []string) int {
	return subcommands.Run(application(p), fixflagpos.FixSubcommands(args))
}

var badArgsTag = errtag.Make("Bad arguments given", true)
