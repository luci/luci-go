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
			{"invocation"},
			{"invocation", "-h"},
			{"invocation", "get", "-h"},
			{"help", "invocation", "get"},
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
			{"notify"},
			{"notify", "-h"},
			{"notify", "builder", "-h"},
			{"notify", "builder", "list", "-h"},
			{"notify", "alert-group", "-h"},
			{"notify", "alert-group", "list", "-h"},
			{"notify", "alert-group", "get", "-h"},
		}

		for _, rawArgs := range helpInvocations {
			cApp := &capturedApp{Application: getApplication()}
			code := runApp(cApp, rawArgs)
			combined := cApp.out.String() + cApp.err.String()
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, strings.Count(combined, base.AgentSkillHint), should.Equal(1))
		}
	})
}

func TestUnknownCommands(t *testing.T) {
	t.Parallel()

	run := func(args ...string) (int, string, string) {
		cApp := &capturedApp{Application: getApplication()}
		code := runApp(cApp, args)
		return code, cApp.out.String(), cApp.err.String()
	}

	ftt.Run(`top-level unknown commands emit actionable cheat-sheet and exit 2`, t, func(t *ftt.Test) {
		cases := [][]string{
			{"resultdb"},
			{"resultdb", "test-results"},
			{"resultdb", "--help"},
			{"help", "resultdb"},
			{"results"},
			{"result"},
			{"result", "query"},
			{"foobar"},
		}
		for _, args := range cases {
			code, out, errOut := run(args...)
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, out, should.BeEmpty)
			assert.Loosely(t, errOut, should.ContainSubstring("luci: unknown command"))
			assert.Loosely(t, errOut, should.ContainSubstring("Common luci commands:"))
			assert.Loosely(t, errOut, should.ContainSubstring("luci verdict list"))
			assert.Loosely(t, errOut, should.ContainSubstring("luci verdict get"))
			assert.Loosely(t, errOut, should.ContainSubstring("luci test-result get"))
			assert.Loosely(t, errOut, should.ContainSubstring("luci artifact"))
			assert.Loosely(t, errOut, should.ContainSubstring("Run 'luci help' for usage."))
		}

		t.Run(`resultdb/results/result include ResultDB top-level note`, func(t *ftt.Test) {
			for _, cmd := range []string{"resultdb", "results", "result"} {
				code, _, errOut := run(cmd)
				assert.Loosely(t, code, should.Equal(2))
				assert.Loosely(t, errOut, should.ContainSubstring("ResultDB commands are exposed directly at the top level"))
			}
		})

		t.Run(`ants command explains direct AnTS ID support`, func(t *ftt.Test) {
			code, out, errOut := run("ants", "invocation", "get", "I46300010666440807")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, out, should.BeEmpty)
			assert.Loosely(t, errOut, should.ContainSubstring("There is no 'ants' subcommand"))
			assert.Loosely(t, errOut, should.ContainSubstring("luci invocation get I46300010666440807"))
			assert.Loosely(t, errOut, should.ContainSubstring("Common luci commands:"))

			code, _, errOut = run("ants", "verdicts", "list", "I46300010666440807")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("There is no 'ants' subcommand"))
			assert.Loosely(t, errOut, should.ContainSubstring("luci verdict list I46300010666440807"))

			code, _, errOut = run("ants")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("luci invocation get <I...>"))
			assert.Loosely(t, errOut, should.ContainSubstring("luci verdict list <I...>"))
		})

		t.Run(`top-level ambiguous prefix and component names suggest closest commands`, func(t *ftt.Test) {
			code, _, errOut := run("auth")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("Did you mean: auth-info, auth-login, auth-logout?"))

			code, _, errOut = run("login")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("Did you mean: auth-login?"))

			code, _, errOut = run("unit")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("Did you mean: work-unit?"))
		})
	})

	ftt.Run(`nested unknown commands emit targeted and fuzzy hints`, t, func(t *ftt.Test) {
		t.Run(`luci invocation <id> suggests inserting get`, func(t *ftt.Test) {
			code, out, errOut := run("invocation", "I43300010665891529")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, out, should.BeEmpty)
			assert.Loosely(t, errOut, should.ContainSubstring("Hint: 'luci invocation' only supports 'get'. Try:\n  luci invocation get I43300010665891529"))
		})

		t.Run(`leading flags on nested parent groups reach actionable subcommand hints`, func(t *ftt.Test) {
			code, out, errOut := run("invocation", "-invocationid", "I43300010665891529")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, out, should.BeEmpty)
			assert.Loosely(t, errOut, should.NotContainSubstring("flag provided but not defined"))
			assert.Loosely(t, errOut, should.ContainSubstring("Hint: 'luci invocation' only supports 'get'. Try:\n  luci invocation get -invocationid I43300010665891529"))

			code, out, errOut = run("analysis", "rule", "-bugid", "553974454")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, out, should.BeEmpty)
			assert.Loosely(t, errOut, should.NotContainSubstring("flag provided but not defined"))
			assert.Loosely(t, errOut, should.ContainSubstring("Hint: 'luci analysis rule' only supports 'get'. Try:\n  luci analysis rule get -bugid 553974454"))

			code, out, errOut = run("verdict", "-invocationid", "build-123")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, out, should.BeEmpty)
			assert.Loosely(t, errOut, should.NotContainSubstring("flag provided but not defined"))
			assert.Loosely(t, errOut, should.ContainSubstring("Available subcommands for 'luci verdict': get, list"))
		})

		t.Run(`luci invocation <non-id verb> lists available subcommands`, func(t *ftt.Test) {
			code, _, errOut := run("invocation", "list")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("Available subcommands for 'luci invocation': get"))
			assert.Loosely(t, errOut, should.NotContainSubstring("luci invocation get list"))
		})

		t.Run(`luci test-results list and luci test-result list point to luci verdict list`, func(t *ftt.Test) {
			for _, args := range [][]string{
				{"test-results", "list"},
				{"test-result", "list"},
				{"help", "test-result", "list"},
			} {
				code, out, errOut := run(args...)
				assert.Loosely(t, code, should.Equal(2))
				assert.Loosely(t, out, should.BeEmpty)
				assert.Loosely(t, errOut, should.ContainSubstring("luci verdict list -invocationid <INV>"))
				assert.Loosely(t, errOut, should.ContainSubstring("Available subcommands for 'luci test-result': get, artifact"))
			}
		})

		t.Run(`nested subcommand typos and component names suggest closest subcommands`, func(t *ftt.Test) {
			code, _, errOut := run("build", "l")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("Did you mean: log, list?"))
			assert.Loosely(t, errOut, should.ContainSubstring("Available subcommands for 'luci build': get, list, log"))

			code, _, errOut = run("artifact", "show")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("Available subcommands for 'luci artifact': list, get, head, tail, compare"))

			code, _, errOut = run("analysis", "history")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("Did you mean: test-history?"))

			code, _, errOut = run("notify", "group")
			assert.Loosely(t, code, should.Equal(2))
			assert.Loosely(t, errOut, should.ContainSubstring("Did you mean: alert-group?"))
		})
	})

	ftt.Run(`working Levenshtein plural commands remain unaffected`, t, func(t *ftt.Test) {
		for _, args := range [][]string{
			{"artifacts", "list", "-h"},
			{"verdicts", "list", "-h"},
			{"test-results", "get", "-h"},
		} {
			code, _, _ := run(args...)
			assert.Loosely(t, code, should.BeZero)
		}
	})
}
