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
			"CONTEXT SHORTCUT ('-'):\n" +
			"  Commands support '-' as a target argument to reuse recently accessed resources\n" +
			"  (invocation, work unit, test result, verdict) without re-specifying them.\n" +
			"  Trailing arguments after '-' override trailing components in the hierarchy.\n\n" +
			"  Typical Investigation Workflow Examples:\n" +
			"    1. Fetch a test verdict by URL or name:\n" +
			"       $ luci verdict get https://ci.chromium.org/ui/test-investigate/invocations/build-123/...\n\n" +
			"    2. Inspect a failed run using '-' to reuse the verdict/invocation context:\n" +
			"       $ luci test-result get - 0efc0e5a-00528\n\n" +
			"    3. List or download test result artifacts using '-' to reuse the test result:\n" +
			"       $ luci test-result artifact list -\n" +
			"       $ luci test-result artifact get - output.log\n\n" +
			"    4. Inspect a parent/ancestor work unit using '-' to reuse the invocation:\n" +
			"       $ luci work-unit get - ants-wu83500269020198004\n\n" +
			"    5. List or download work unit artifacts using '-' to reuse the work unit:\n" +
			"       $ luci work-unit artifact list -\n" +
			"       $ luci work-unit artifact get - subprocess-test_result.xml.gz",

		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},

		Commands: []*subcommands.Command{
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
