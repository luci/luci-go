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
	"testing"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNormalizeAppArgs(t *testing.T) {
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
			subcommands.CmdHelp,
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
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"get", "-show-artifacts", "-h"}), should.Match([]string{"help", "get"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"get", "-h", "name"}), should.Match([]string{"help", "get"}))
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"list", "name", "--help"}), should.Match([]string{"help", "list"}))
		})

		t.Run(`nested parent commands preserve args for child app`, func(t *ftt.Test) {
			assert.Loosely(t, NormalizeAppArgs(cmds, []string{"test-result", "-h"}), should.Match([]string{"test-result", "-h"}))
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
