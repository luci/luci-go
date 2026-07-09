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

package testresult

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCmdTestResult(t *testing.T) {
	t.Parallel()

	ftt.Run(`Cmd`, t, func(t *ftt.Test) {
		cmd := Cmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("test-result <subcommand>"))

		getCmd := GetCmd(nil)
		assert.Loosely(t, getCmd, should.NotBeNil)
		assert.Loosely(t, getCmd.UsageLine, should.Equal("get <name>"))
	})
}
