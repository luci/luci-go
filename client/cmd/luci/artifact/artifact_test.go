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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestArtifactCmds(t *testing.T) {
	t.Parallel()

	ftt.Run(`Artifact commands`, t, func(t *ftt.Test) {
		trCmd := TestResultArtifactCmd(nil)
		assert.Loosely(t, trCmd, should.NotBeNil)
		assert.Loosely(t, trCmd.UsageLine, should.Equal("artifact <subcommand>"))

		wuCmd := WorkUnitArtifactCmd(nil)
		assert.Loosely(t, wuCmd, should.NotBeNil)
		assert.Loosely(t, wuCmd.UsageLine, should.Equal("artifact <subcommand>"))

		listCmd := ListCmd(nil, ParentTypeTestResult)
		assert.Loosely(t, listCmd, should.NotBeNil)

		getCmd := GetCmd(nil, ParentTypeTestResult)
		assert.Loosely(t, getCmd, should.NotBeNil)

		headCmd := HeadCmd(nil, ParentTypeTestResult)
		assert.Loosely(t, headCmd, should.NotBeNil)

		tailCmd := TailCmd(nil, ParentTypeTestResult)
		assert.Loosely(t, tailCmd, should.NotBeNil)

		topCmd := Cmd(nil)
		assert.Loosely(t, topCmd, should.NotBeNil)
		assert.Loosely(t, topCmd.UsageLine, should.Equal("artifact <subcommand>"))
		run := topCmd.CommandRun()
		assert.Loosely(t, run.Run(nil, []string{}, nil), should.Equal(0))
		assert.Loosely(t, run.Run(nil, []string{"--help"}, nil), should.Equal(0))
		assert.Loosely(t, run.Run(nil, []string{"help"}, nil), should.Equal(0))
		assert.Loosely(t, run.Run(nil, []string{"list", "--help"}, nil), should.Equal(0))
		assert.Loosely(t, run.Run(nil, []string{"get", "--help"}, nil), should.Equal(0))
		assert.Loosely(t, run.Run(nil, []string{"head", "--help"}, nil), should.Equal(0))
		assert.Loosely(t, run.Run(nil, []string{"tail", "--help"}, nil), should.Equal(0))

		unknownListCmd := ListCmd(nil, ParentTypeUnknown)
		assert.Loosely(t, unknownListCmd, should.NotBeNil)
		assert.Loosely(t, unknownListCmd.ShortDesc, should.Equal("List artifacts for a work unit or test result"))

		unknownGetCmd := GetCmd(nil, ParentTypeUnknown)
		assert.Loosely(t, unknownGetCmd, should.NotBeNil)
		assert.Loosely(t, unknownGetCmd.ShortDesc, should.Equal("Get a work unit or test result artifact"))

		unknownHeadCmd := HeadCmd(nil, ParentTypeUnknown)
		assert.Loosely(t, unknownHeadCmd, should.NotBeNil)
		assert.Loosely(t, unknownHeadCmd.ShortDesc, should.Equal("Print the first N lines of a work unit or test result artifact"))

		unknownTailCmd := TailCmd(nil, ParentTypeUnknown)
		assert.Loosely(t, unknownTailCmd, should.NotBeNil)
		assert.Loosely(t, unknownTailCmd.ShortDesc, should.Equal("Print the last N lines of a work unit or test result artifact"))
	})
}
