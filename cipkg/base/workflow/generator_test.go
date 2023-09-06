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

package workflow

import (
	"context"
	"testing"

	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
	"go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerator(t *testing.T) {
	Convey("Test Generator", t, func() {
		ctx := context.Background()

		g := &Generator{
			Name: "first",
			Dependencies: []generators.Dependency{
				{Generator: &Generator{Name: "second"}, Type: generators.DepsBuildHost, Runtime: true},
			},
		}

		a, err := g.Generate(ctx, generators.Platforms{})
		So(err, ShouldBeNil)

		So(a.Name, ShouldEqual, "first")
		So(a.Deps, ShouldHaveLength, 1)
		So(a.Deps[0].Name, ShouldEqual, "second")
		So(a.Metadata.RuntimeDeps, ShouldHaveLength, 1)
		So(a.Metadata.RuntimeDeps[0], assertions.ShouldResembleProto, a.Deps[0])
		cmd := testutils.Assert[*core.Action_Command](t, a.Spec)
		So(cmd.Command.Env, ShouldEqual, []string{"depsBuildHost={{.second}}", "second={{.second}}"})
	})

}
