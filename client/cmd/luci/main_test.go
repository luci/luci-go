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
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type capturedApp struct {
	*cli.Application
	out bytes.Buffer
	err bytes.Buffer
}

func (a *capturedApp) GetOut() io.Writer { return &a.out }
func (a *capturedApp) GetErr() io.Writer { return &a.err }

func TestGetApplication(t *testing.T) {
	t.Parallel()

	ftt.Run(`getApplication`, t, func(t *ftt.Test) {
		app := getApplication()
		assert.Loosely(t, app, should.NotBeNil)
		assert.Loosely(t, app.Name, should.Equal("luci"))
		assert.Loosely(t, len(app.Commands), should.BeGreaterThan(2))
		assert.Loosely(t, app.Title, should.ContainSubstring(base.AgentSkillHint))
		assert.Loosely(t, app.Title, should.ContainSubstring("go/luci-skill"))

		helpInvocations := [][]string{
			{},
			{"help"},
			{"-h"},
			{"--help"},
			{"-help"},
			{"ids", "-h"},
			{"help", "ids"},
			{"verdict"},
			{"verdict", "-h"},
			{"verdict", "--help"},
			{"verdict", "-h", "-invocationid", "build-123"},
			{"verdict", "-invocationid", "build-123", "-h"},
			{"verdict", "help"},
			{"verdict", "list", "-h"},
			{"help", "verdict", "list"},
			{"build", "log", "-h"},
			{"build", "log", "-buildid", "123", "-h"},
			{"build", "log", "tail", "-h"},
			{"help", "build", "log", "tail"},
			{"test-result", "artifact", "-h"},
			{"test-result", "artifact", "list", "-h"},
			{"analysis", "rule", "-h"},
			{"analysis", "rule", "get", "-h"},
		}

		for _, rawArgs := range helpInvocations {
			cApp := &capturedApp{Application: getApplication()}
			normArgs := base.NormalizeAppArgs(cApp.Commands, rawArgs)
			code := subcommands.Run(cApp, normArgs)
			combined := cApp.out.String() + cApp.err.String()
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, strings.Count(combined, base.AgentSkillHint), should.Equal(1))
		}
	})
}
