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
	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/client/cmd/luci/testresult"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/client/cmd/luci/workunit"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func getApplication() *cli.Application {
	authOpts := chromeinfra.DefaultAuthOptions()
	af := base.NewAuthFlags()

	return &cli.Application{
		Name: "luci",
		Title: "Unified CLI tool to access LUCI resources.\n\n" +
			"Commands are stateless and require explicit resource ID flags (-invocationid, -testid, -resultid, -workunitid, -artifactid).\n" +
			"Use 'luci ids <url>' to extract IDs from URLs (Milo UI, AnTS/ATI) or canonical resource names.\n\n" +
			"Typical Investigation Workflow Examples:\n" +
			"  1. Extract IDs from a Milo or ATI test URL:\n" +
			"     $ luci ids https://ci.chromium.org/ui/test-investigate/invocations/build-123/...\n" +
			"     $ luci ids https://android-build.corp.google.com/test_investigate/invocation/I.../test/TR...\n\n" +
			"  2. Inspect a test verdict:\n" +
			"     $ luci verdict get -invocationid build-123 -testid ninja://chrome/test\n\n" +
			"  3. Inspect an individual test result:\n" +
			"     $ luci test-result get -invocationid build-123 -testid ninja://chrome/test -resultid 0\n\n" +
			"  4. List and inspect test result or work unit artifacts:\n" +
			"     $ luci test-result artifact list -invocationid build-123 -testid ninja://chrome/test -resultid 0\n" +
			"     $ luci test-result artifact get -invocationid build-123 -testid ninja://chrome/test -resultid 0 -artifactid output.log\n" +
			"     $ luci work-unit get -invocationid build-123 -workunitid run-tests\n" +
			"     $ luci work-unit artifact list -invocationid build-123 -workunitid run-tests",

		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},

		Commands: []*subcommands.Command{
			ids.Cmd(af),
			artifact.Cmd(af),
			testresult.Cmd(af),
			verdict.Cmd(af),
			workunit.Cmd(af),

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
	app := getApplication()
	args := base.NormalizeAppArgs(app.Commands, os.Args[1:])
	os.Exit(subcommands.Run(app, args))
}
