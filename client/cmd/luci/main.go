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
	"go.chromium.org/luci/client/cmd/luci/analysis"
	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/build"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/client/cmd/luci/module"
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
			"Commands are stateless and accept either a positional URL/ID <target> (Milo UI, LUCI Analysis, AnTS/ATI, Buganizer, or canonical resource names) or explicit resource ID flags (-bugid, -project, -ruleid, -clusteralgorithm, -clusterid, -buildid, -invocationid, -modulename, -testid, -resultid, -workunitid, -artifactid).\n" +
			"When a positional <target> is passed, derived IDs are printed automatically so follow-up commands can use explicit flags.\n\n" +
			base.AgentSkillHint + "\n\n" +
			"Typical Investigation Workflow Examples:\n" +
			"  1. Resolve a LUCI Analysis bug or rule to variant breakdown & failing invocations:\n" +
			"     $ luci analysis failures b/553974454\n" +
			"     $ luci analysis failures -bugid 553974454\n\n" +
			"  2. Inspect a test verdict or build directly from a Milo or ATI URL:\n" +
			"     $ luci verdict get https://ci.chromium.org/ui/test-investigate/invocations/build-123/...\n" +
			"     $ luci verdict get https://android-build.corp.google.com/test_investigate/invocation/I.../test/TR...\n" +
			"     $ luci build get https://ci.chromium.org/ui/b/8738491827364512345\n\n" +
			"  3. Search or inspect Buildbucket builds, their status, summary, failed steps, and step logs:\n" +
			"     $ luci build list -builder \"chromium/ci/Linux UBSan Tests\" -n 5\n" +
			"     $ luci build get -buildid 8738491827364512345\n" +
			"     $ luci build log tail -buildid 8738491827364512345 -step compile -n 50\n\n" +
			"  4. List test verdicts in an invocation:\n" +
			"     $ luci verdict list -invocationid build-123\n\n" +
			"  5. Inspect a module and its shards/errors:\n" +
			"     $ luci module get -invocationid ants-i123 -modulename CellBroadcastReceiverMTS\n\n" +
			"  6. Inspect a test verdict:\n" +
			"     $ luci verdict get -invocationid build-123 -testid ninja://chrome/test\n\n" +
			"  7. Inspect an individual test result:\n" +
			"     $ luci test-result get -invocationid build-123 -testid ninja://chrome/test -resultid 0\n\n" +
			"  8. List and inspect test result or work unit artifacts:\n" +
			"     $ luci artifact list -invocationid build-123 -workunitid run-tests\n" +
			"     $ luci artifact get -invocationid build-123 -workunitid run-tests -artifactid test.xml\n" +
			"     $ luci artifact list -invocationid build-123 -testid ninja://chrome/test -resultid 0\n" +
			"     $ luci artifact get -invocationid build-123 -testid ninja://chrome/test -resultid 0 -artifactid output.log\n" +
			"     $ luci work-unit get -invocationid build-123 -workunitid run-tests",

		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},

		Commands: []*subcommands.Command{
			ids.Cmd(af),
			analysis.Cmd(af),
			artifact.Cmd(af),
			build.Cmd(af),
			module.Cmd(af),
			testresult.Cmd(af),
			verdict.Cmd(af),
			workunit.Cmd(af),

			subcommands.Section("Authentication\n"),
			authcli.SubcommandInfo(authOpts, "auth-info", false),
			authcli.SubcommandLogin(authOpts, "auth-login", false),
			authcli.SubcommandLogout(authOpts, "auth-logout", false),

			subcommands.Section("Other\n"),
			base.HelpCmd(),
		},
	}
}

func main() {
	app := getApplication()
	args := base.NormalizeAppArgs(app.Commands, os.Args[1:])
	os.Exit(subcommands.Run(app, args))
}
