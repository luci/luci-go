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

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProcessCommand(t *testing.T) {
	Convey("Test action processor for cipd", t, func() {
		ap := NewActionProcessor("", testutils.NewMockPackageManage(""))

		Convey("ok", func() {
			cmd := &core.ActionCommand{
				Args: []string{"bin", "arg1", "arg2", "{{.something}}/{{.something}}"},
				Env:  []string{"env1=var1", "env2=var2", "env3={{.something}}"},
			}

			pkg, err := ap.Process(&core.Action{
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
			So(err, ShouldBeNil)

			So(pkg.BuildDependencies, ShouldHaveLength, 1)
			So(pkg.BuildDependencies[0].Action.Name, ShouldEqual, "something")
			depOut := pkg.BuildDependencies[0].Handler.OutputDirectory()
			So(pkg.Derivation.Inputs, ShouldHaveLength, 1)
			So(pkg.Derivation.Args, ShouldEqual, []string{"bin", "arg1", "arg2", depOut + "/" + depOut})
			So(pkg.Derivation.Env, ShouldEqual, []string{"env1=var1", "env2=var2", "env3=" + depOut})
			So(pkg.RuntimeDependencies, ShouldHaveLength, 1)
			So(pkg.RuntimeDependencies[0].Action.Name, ShouldEqual, "else")
		})

		Convey("invalid template", func() {
			cmd := &core.ActionCommand{
				Args: []string{"bin", "arg1", "arg2", "{{something}}"},
			}

			_, err := ap.Process(&core.Action{
				Name: "url",
				Deps: []*core.Action{
					{Name: "something", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Command{Command: cmd},
			})
			So(err, ShouldNotBeNil)
		})

		Convey("unknown key", func() {
			cmd := &core.ActionCommand{
				Env: []string{"{{.else}}"},
			}

			_, err := ap.Process(&core.Action{
				Name: "url",
				Deps: []*core.Action{
					{Name: "something", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Command{Command: cmd},
			})
			So(err, ShouldNotBeNil)
		})
	})
}
