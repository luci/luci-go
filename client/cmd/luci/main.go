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
	"fmt"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/cmd/luci/analysis"
	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/build"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/client/cmd/luci/invocation"
	"go.chromium.org/luci/client/cmd/luci/module"
	"go.chromium.org/luci/client/cmd/luci/notify"
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
			"Commands are stateless and accept either a positional URL/ID <target> (Milo UI, LUCI Analysis, LUCI Notify, AnTS/ATI, Buganizer, or canonical resource names) or explicit resource ID flags (-bugid, -project, -ruleid, -clusteralgorithm, -clusterid, -buildid, -invocationid, -modulename, -testid, -resultid, -workunitid, -artifactid, -rotation, -id).\n" +
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
			"     $ luci build list -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529\n" +
			"     $ luci build get -buildid 8738491827364512345\n" +
			"     $ luci build log tail -buildid 8738491827364512345 -step compile -n 50\n\n" +
			"  4. Inspect an invocation's build, branch, target, sources, and test verdicts:\n" +
			"     $ luci invocation get -invocationid ants-i123\n" +
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
			"     $ luci work-unit get -invocationid build-123 -workunitid run-tests\n\n" +
			"  9. Query LUCI Notify builder statuses and gardener rotation alert groups:\n" +
			"     $ luci notify builder list -rotation chrome_browser_release -status FAILURE\n" +
			"     $ luci notify alert-group list -rotation chrome-browser-release\n" +
			"     $ luci notify alert-group get -rotation chrome-browser-release -id 12345",

		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},

		Commands: []*subcommands.Command{
			ids.Cmd(af),
			analysis.Cmd(af),
			artifact.Cmd(af),
			build.Cmd(af),
			invocation.Cmd(af),
			module.Cmd(af),
			notify.Cmd(af),
			testresult.Cmd(af),
			verdict.Cmd(af),
			workunit.Cmd(af),

			subcommands.Section("Authentication\n"),
			authcli.SubcommandInfo(authOpts, "auth-info", false),
			authcli.SubcommandLogin(authOpts, "auth-login", false),
			authcli.SubcommandLogout(authOpts, "auth-logout", false),

			subcommands.Section("Other\n"),
			base.HelpCmdWithHint(topLevelUnknownCommandHint),
		},
	}
}

func topLevelUnknownCommandHint(a subcommands.Application, unknownCmd string, rest []string) string {
	var parts []string
	lower := strings.ToLower(strings.TrimSpace(unknownCmd))

	switch lower {
	case "ants":
		if len(rest) > 0 && rest[0] != "" {
			if c := subcommands.FindNearestCommand(a, rest[0]); c != nil && c.Name() != "" && c.Name() != "help" {
				quoted := make([]string, 0, len(rest))
				quoted = append(quoted, base.ShellQuoteIfNeeded(c.Name()))
				for _, arg := range rest[1:] {
					quoted = append(quoted, base.ShellQuoteIfNeeded(arg))
				}
				parts = append(parts,
					"Note: There is no 'ants' subcommand; AnTS IDs (I..., TR..., WU...) and URLs work directly without it. Try:\n"+
						"  luci "+strings.Join(quoted, " "))
			}
		}
		if len(parts) == 0 {
			parts = append(parts,
				"Note: There is no 'ants' subcommand. Pass AnTS IDs (I..., TR..., WU...) or URLs directly to top-level commands, e.g.:\n"+
					"  luci invocation get <I...>\n"+
					"  luci verdict list <I...>\n"+
					"  luci work-unit get <WU...>")
		}
	case "resultdb", "results", "result":
		parts = append(parts,
			fmt.Sprintf("Note: ResultDB commands are exposed directly at the top level (there is no '%s' prefix).", lower))
	default:
		if suggestions := base.SuggestCommands(a.GetCommands(), unknownCmd); len(suggestions) > 0 {
			parts = append(parts, fmt.Sprintf("Did you mean: %s?", strings.Join(suggestions, ", ")))
		}
	}

	parts = append(parts,
		"Common luci commands:\n"+
			"  luci ids <URL|ID>                                      Decompose a URL or ID into resource flags\n"+
			"  luci verdict list -invocationid <INV>                  List failing test verdicts in an invocation\n"+
			"  luci verdict get -invocationid <INV> -testid <TEST>    Inspect a test verdict (retries & exonerations)\n"+
			"  luci test-result get -invocationid <INV> -testid <TEST> -resultid <RES>\n"+
			"                                                         Inspect an individual test result attempt\n"+
			"  luci artifact <list|get|head|tail|compare> ...         List and inspect test or work unit artifacts\n"+
			"  (Also: build, analysis, invocation, module, work-unit, notify)")

	return strings.Join(parts, "\n\n")
}

func runApp(a subcommands.Application, args []string) int {
	return base.RunApp(a, args, topLevelUnknownCommandHint)
}

func main() {
	os.Exit(runApp(getApplication(), os.Args[1:]))
}
