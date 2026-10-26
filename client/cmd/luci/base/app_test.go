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

package base

import (
	"bytes"
	"strings"
	"testing"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNormalizeAppArgs(t *testing.T) {
	t.Parallel()

	ftt.Run(`NormalizeAppArgs`, t, func(t *ftt.Test) {
		dummyLeafCmd := &subcommands.Command{
			UsageLine: "get <name>",
			ShortDesc: "get details",
		}
		dummyParentCmd := &subcommands.Command{
			UsageLine: "test-result <subcommand>",
			ShortDesc: "manage test results",
		}
		dummyArtifactParentCmd := &subcommands.Command{
			UsageLine: "artifact <subcommand>",
			ShortDesc: "manage artifacts",
		}
		dummyListLeafCmd := &subcommands.Command{
			UsageLine: "list <name>",
			ShortDesc: "list items",
		}
		cmds := []*subcommands.Command{
			dummyLeafCmd,
			dummyParentCmd,
			dummyArtifactParentCmd,
			dummyListLeafCmd,
			HelpCmd(),
		}

		t.Run(`empty args becomes help`, func(t *ftt.Test) {
			assert.Loosely(t, NormalizeAppArgs(cmds, nil), should.Match([]string{"help"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{}), should.Match([]string{"help"}))
		})

		t.Run(`top-level help flags`, func(t *ftt.Test) {
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"-h"}), should.Match([]string{"help"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"--help"}), should.Match([]string{"help"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"-help"}), should.Match([]string{"help"}))
		})

		t.Run(`leaf commands with trailing help flag or positional args`, func(t *ftt.Test) {
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"get", "-h"}), should.Match([]string{"help", "get"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"get", "name", "-h"}), should.Match([]string{"help", "get"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"get", "-show-metadata", "-h"}), should.Match([]string{"help", "get"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"get", "-h", "name"}), should.Match([]string{"help", "get"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"list", "name", "--help"}), should.Match([]string{"help", "list"}))
		})

		t.Run(`nested parent commands translate immediate or flag-accompanied help and preserve child args`, func(t *ftt.Test) {
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "-h"}), should.Match([]string{"test-result", "help"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "--help"}), should.Match([]string{"test-result", "help"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "-h", "-invocationid", "build-123"}), should.Match([]string{"test-result", "help"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "-invocationid", "build-123", "-h"}), should.Match([]string{"test-result", "help"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "-h", "get"}), should.Match([]string{"test-result", "help", "get"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "-h", "get", "-invocationid", "build-123"}), should.Match([]string{"test-result", "help", "get"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "get", "-h"}), should.Match([]string{"test-result", "get", "-h"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "get", "name", "-h"}), should.Match([]string{"test-result", "get", "name", "-h"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"artifact", "list", "-h"}), should.Match([]string{"artifact", "list", "-h"}))
		})

		t.Run(`normal commands unchanged`, func(t *ftt.Test) {
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"get", "-"}), should.Match([]string{"get", "-"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"list", "name"}), should.Match([]string{"list", "name"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "get", "-"}), should.Match([]string{"test-result", "get", "-"}))
		})
	})
}

func TestHelpCmdAndSkillHint(t *testing.T) {
	t.Parallel()

	ftt.Run(`HelpCmd and RunSubcommandApp output AgentSkillHint`, t, func(t *ftt.Test) {
		assert.Loosely(t, FormatTitleWithSkillHint(""), should.Equal(AgentSkillHint))
		titled := FormatTitleWithSkillHint("Subcommand title")
		assert.Loosely(t, titled, should.ContainSubstring("Subcommand title"))
		assert.Loosely(t, titled, should.ContainSubstring(AgentSkillHint))
		assert.Loosely(t, FormatTitleWithSkillHint(titled), should.Equal(titled))

		leafCmd := &subcommands.Command{
			UsageLine: "get -id <id>",
			ShortDesc: "get item",
			LongDesc:  "Get an item by ID.",
			CommandRun: func() subcommands.CommandRun {
				r := &dummyRun{}
				r.Flags.String("id", "", "item ID")
				return r
			},
		}

		var outBuf, errBuf bytes.Buffer
		parent := &subcommandApp{
			Application: &cli.Application{Name: "luci"},
			out:         &outBuf,
			err:         &errBuf,
		}

		t.Run(`group help outputs skill hint once to stdout`, func(t *ftt.Test) {
			code := RunSubcommandApp(parent, "luci item", "Item management", []*subcommands.Command{
				leafCmd,
				subcommands.CmdHelp,
			}, []string{"-h"})
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, strings.Count(outBuf.String(), AgentSkillHint), should.Equal(1))
			assert.Loosely(t, outBuf.String(), should.ContainSubstring("go/luci-skill"))
		})

		t.Run(`leaf help outputs skill hint once to stderr`, func(t *ftt.Test) {
			code := RunSubcommandApp(parent, "luci item", "Item management", []*subcommands.Command{
				leafCmd,
				subcommands.CmdHelp,
			}, []string{"get", "-h"})
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, strings.Count(errBuf.String(), AgentSkillHint), should.Equal(1))
			assert.Loosely(t, errBuf.String(), should.ContainSubstring("Get an item by ID."))
		})

		t.Run(`extra args on leaf help exits with code 2`, func(t *ftt.Test) {
			code := RunSubcommandApp(parent, "luci item", "Item management", []*subcommands.Command{
				leafCmd,
				subcommands.CmdHelp,
			}, []string{"help", "get", "extra"})
			assert.Loosely(t, code, should.Equal(2))
		})
	})
}

type dummyRun struct {
	subcommands.CommandRunBase
}

func (r *dummyRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return 0
}
