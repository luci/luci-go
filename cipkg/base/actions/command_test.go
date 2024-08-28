// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package actions

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
)

func TestProcessCommand(t *testing.T) {
	ftt.Run("Test action processor for cipd", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		t.Run("ok", func(t *ftt.Test) {
			cmd := &core.ActionCommand{
				Args: []string{"bin", "arg1", "arg2", "{{.something}}/{{.something}}"},
				Env:  []string{"env1=var1", "env2=var2", "env3={{.something}}"},
			}

			pkg, err := ap.Process("", pm, &core.Action{
				Name: "url",
				Metadata: &core.Action_Metadata{
					RuntimeDeps: []*core.Action{
						{Name: "else", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
					},
				},
				Deps: []*core.Action{
					{Name: "something", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Command{Command: cmd},
			})
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, pkg.BuildDependencies, should.HaveLength(1))
			assert.Loosely(t, pkg.BuildDependencies[0].Action.Name, should.Equal("something"))
			depOut := pkg.BuildDependencies[0].Handler.OutputDirectory()
			assert.Loosely(t, pkg.Derivation.Inputs, should.HaveLength(1))
			assert.Loosely(t, pkg.Derivation.Args, should.Match([]string{"bin", "arg1", "arg2", depOut + "/" + depOut}))
			assert.Loosely(t, pkg.Derivation.Env, should.Match([]string{"env1=var1", "env2=var2", "env3=" + depOut}))
			assert.Loosely(t, pkg.RuntimeDependencies, should.HaveLength(1))
			assert.Loosely(t, pkg.RuntimeDependencies[0].Action.Name, should.Equal("else"))
		})

		t.Run("invalid template", func(t *ftt.Test) {
			cmd := &core.ActionCommand{
				Args: []string{"bin", "arg1", "arg2", "{{something}}"},
			}

			_, err := ap.Process("", pm, &core.Action{
				Name: "url",
				Deps: []*core.Action{
					{Name: "something", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Command{Command: cmd},
			})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("unknown key", func(t *ftt.Test) {
			cmd := &core.ActionCommand{
				Env: []string{"{{.else}}"},
			}

			_, err := ap.Process("", pm, &core.Action{
				Name: "url",
				Deps: []*core.Action{
					{Name: "something", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Command{Command: cmd},
			})
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
