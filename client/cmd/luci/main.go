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

package main

import (
	"context"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/testresult"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func getApplication() *cli.Application {
	authOpts := chromeinfra.DefaultAuthOptions()
	af := base.NewAuthFlags()

	return &cli.Application{
		Name:  "luci",
		Title: "Unified CLI tool to access LUCI resources.",

		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},

		Commands: []*subcommands.Command{
			testresult.Cmd(af),
			verdict.Cmd(af),

			subcommands.Section("Authentication\n"),
			authcli.SubcommandInfo(authOpts, "auth-info", false),
			authcli.SubcommandLogin(authOpts, "auth-login", false),
			authcli.SubcommandLogout(authOpts, "auth-logout", false),

			subcommands.Section("Other\n"),
			subcommands.CmdHelp,
		},
	}
}

func main() {
	os.Exit(subcommands.Run(getApplication(), nil))
}
