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

// Package cli contains command line interface for lucicfg tool.
package cli

import (
	"context"
	"os"

	"github.com/maruel/subcommands"
	"go.starlark.net/resolve"

	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/lucicfg"
	"go.chromium.org/luci/lucicfg/cli/base"
	"go.chromium.org/luci/lucicfg/cli/cmds/generate"
	"go.chromium.org/luci/lucicfg/cli/cmds/validate"
)

// Main runs the lucicfg CLI.
func Main(params base.Parameters, args []string) int {
	// Enable not-yet-standard Starlark features.
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowFloat = true
	resolve.AllowSet = true

	return subcommands.Run(GetApplication(params), fixflagpos.FixSubcommands(args))
}

// GetApplication returns lucicfg cli.Application.
func GetApplication(params base.Parameters) *cli.Application {
	return &cli.Application{
		Name:  "lucicfg",
		Title: "LUCI config generator (" + lucicfg.UserAgent + ")",

		Context: func(ctx context.Context) context.Context {
			loggerConfig := gologger.LoggerConfig{
				Format: `[P%{pid} %{time:15:04:05.000} %{shortfile} %{level:.1s}] %{message}`,
				Out:    os.Stderr,
			}
			return loggerConfig.Use(ctx)
		},

		Commands: []*subcommands.Command{
			subcommands.Section("Config generation\n"),
			generate.Cmd(params),
			validate.Cmd(params),

			subcommands.Section("Authentication for LUCI Config\n"),
			authcli.SubcommandInfo(params.AuthOptions, "auth-info", true),
			authcli.SubcommandLogin(params.AuthOptions, "auth-login", false),
			authcli.SubcommandLogout(params.AuthOptions, "auth-logout", false),

			subcommands.Section("Misc\n"),
			subcommands.CmdHelp,
			versioncli.CmdVersion(lucicfg.UserAgent),
		},
	}
}
