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

package artifact

import (
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
)

// TestResultArtifactCmd creates the sub-resource artifact command under `luci test-result artifact`.
func TestResultArtifactCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "artifact <subcommand>",
		ShortDesc: "Manage test result artifacts",
		LongDesc: "Manage artifacts belonging to individual test results in ResultDB.\n\n" +
			"Available subcommands:\n" +
			"  list      List artifacts for a test result\n" +
			"  get       Download or print full content of an artifact\n" +
			"  head      Print first N lines/bytes of an artifact\n" +
			"  tail      Print last N lines/bytes of an artifact",
		CommandRun: func() subcommands.CommandRun {
			return &artifactRun{af: af, parentType: ParentTypeTestResult}
		},
	}
}

// WorkUnitArtifactCmd creates the sub-resource artifact command under `luci work-unit artifact`.
func WorkUnitArtifactCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "artifact <subcommand>",
		ShortDesc: "Manage work unit artifacts",
		LongDesc: "Manage artifacts belonging to work units in ResultDB.\n\n" +
			"Available subcommands:\n" +
			"  list      List artifacts for a work unit\n" +
			"  get       Download or print full content of an artifact\n" +
			"  head      Print first N lines/bytes of an artifact\n" +
			"  tail      Print last N lines/bytes of an artifact",
		CommandRun: func() subcommands.CommandRun {
			return &artifactRun{af: af, parentType: ParentTypeWorkUnit}
		},
	}
}

type artifactRun struct {
	subcommands.CommandRunBase
	af         *base.AuthFlags
	parentType ParentType
}

func (r *artifactRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	var appName, appTitle string
	if r.parentType == ParentTypeTestResult {
		appName = "luci test-result artifact"
		appTitle = "Test result artifact management"
	} else {
		appName = "luci work-unit artifact"
		appTitle = "Work unit artifact management"
	}
	return base.RunSubcommandApp(a, appName, appTitle, []*subcommands.Command{
		ListCmd(r.af, r.parentType),
		GetCmd(r.af, r.parentType),
		HeadCmd(r.af, r.parentType),
		TailCmd(r.af, r.parentType),
		subcommands.CmdHelp,
	}, args)
}

// Cmd returns a top-level command guiding users to work-unit or test-result artifacts.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "artifact <subcommand>",
		ShortDesc: "Manage ResultDB artifacts (scoped to work units or test results)",
		LongDesc: "In ResultDB, artifacts are scoped to either a work unit or an individual test result.\n\n" +
			"Available commands:\n" +
			"  luci work-unit artifact <list|get|head|tail>    Manage artifacts on a work unit\n" +
			"  luci test-result artifact <list|get|head|tail>  Manage artifacts on a test result\n\n" +
			"Tip: Run 'luci ids <url>' to extract the appropriate resource IDs.",
		CommandRun: func() subcommands.CommandRun {
			return &topLevelArtifactRun{}
		},
	}
}

type topLevelArtifactRun struct {
	subcommands.CommandRunBase
}

func (r *topLevelArtifactRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			fmt.Println("In ResultDB, artifacts are scoped to either a work unit or a test result.\n\n" +
				"Available commands:\n" +
				"  luci work-unit artifact <list|get|head|tail>    Manage artifacts on a work unit\n" +
				"  luci test-result artifact <list|get|head|tail>  Manage artifacts on a test result\n\n" +
				"Tip: Run 'luci ids <url>' to extract the appropriate resource IDs.")
			return 0
		}
	}
	fmt.Fprintf(os.Stderr, "In ResultDB, artifacts are scoped to either a work unit or a test result.\n\n"+
		"Please use:\n"+
		"  luci work-unit artifact <list|get|head|tail> -invocationid <id> -workunitid <wu_id> ...\n"+
		"  luci test-result artifact <list|get|head|tail> -invocationid <id> -testid <test_id> -resultid <res_id> ...\n\n"+
		"Tip: Run 'luci ids <url>' to extract the appropriate resource IDs.\n")
	return 1
}
