// Copyright 2019 The LUCI Authors.
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
	"google.golang.org/api/storage/v1"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging/gologger"
)

// Parameters can be used to customize CLI defaults.
type Parameters struct {
	AuthOptions   auth.Options // mostly for client ID and client secret
	DefaultBucket string       // default GCS bucket for packages.
}

// Main runs the lucicfg CLI.
func Main(params Parameters, args []string) int {
	return subcommands.Run(GetApplication(params), fixflagpos.FixSubcommands(args))
}

// GetApplication returns apack app.
func GetApplication(params Parameters) *cli.Application {
	params.AuthOptions.Scopes = append(params.AuthOptions.Scopes, storage.DevstorageReadWriteScope)

	return &cli.Application{
		Name:  "apack",
		Title: "Application packager",

		Context: func(ctx context.Context) context.Context {
			loggerConfig := gologger.LoggerConfig{
				Format: `[P%{pid} %{time:15:04:05.000} %{shortfile} %{level:.1s}] %{message}`,
				Out:    os.Stderr,
			}
			return loggerConfig.Use(ctx)
		},

		Commands: []*subcommands.Command{
			subcommands.Section("Main subcommands\n"),
			Tar(params),
			Pack(params),

			subcommands.Section("Authentication\n"),
			authcli.SubcommandInfo(params.AuthOptions, "auth-info", true),
			authcli.SubcommandLogin(params.AuthOptions, "auth-login", false),
			authcli.SubcommandLogout(params.AuthOptions, "auth-logout", false),

			subcommands.Section("Misc\n"),
			subcommands.CmdHelp,
		},
	}
}
