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
