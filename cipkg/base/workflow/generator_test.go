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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
)

func TestGenerator(t *testing.T) {
	ftt.Run("Test Generator", t, func(t *ftt.Test) {
		ctx := context.Background()

		g := &Generator{
			Name: "first",
			Dependencies: []generators.Dependency{
				{Generator: &Generator{Name: "second"}, Type: generators.DepsBuildHost, Runtime: true},
			},
		}

		a, err := g.Generate(ctx, generators.Platforms{})
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, a.Name, should.Equal("first"))
		assert.Loosely(t, a.Deps, should.HaveLength(1))
		assert.Loosely(t, a.Deps[0].Name, should.Equal("second"))
		assert.Loosely(t, a.Metadata.RuntimeDeps, should.HaveLength(1))
		assert.Loosely(t, a.Metadata.RuntimeDeps[0], should.Match(a.Deps[0]))
		cmd := testutils.Assert[*core.Action_Command](t, a.Spec)
		assert.Loosely(t, cmd.Command.Env, should.Match([]string{"depsBuildHost={{.second}}", "second={{.second}}"}))
	})
}
