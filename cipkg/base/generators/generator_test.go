// Copyright 2026 The LUCI Authors.
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

package generators

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
)

type capturePlatformsGenerator struct {
	plats Platforms
}

func (g *capturePlatformsGenerator) Generate(_ context.Context, plats Platforms) (*core.Action, error) {
	g.plats = plats
	return &core.Action{}, nil
}

func TestDependencyGenerate(t *testing.T) {
	t.Parallel()

	ftt.Run("Dependency.Generate", t, func(t *ftt.Test) {
		ctx := t.Context()
		build := NewPlatform("linux", "amd64")
		host := NewPlatform("linux", "arm64")
		target := NewPlatform("windows", "amd64")
		uni := UniversalPlatform()

		check := func(t *ftt.Test, depType DependencyType, in, want Platforms) {
			t.Helper()
			gen := &capturePlatformsGenerator{}
			dep := &Dependency{Generator: gen, Type: depType}
			_, err := dep.Generate(ctx, in)
			assert.NoErr(t, err)
			assert.That(t, gen.plats.Build, matchPlatform(want.Build))
			assert.That(t, gen.plats.Host, matchPlatform(want.Host))
			assert.That(t, gen.plats.Target, matchPlatform(want.Target))
		}

		t.Run("cross-compile platforms", func(t *ftt.Test) {
			in := Platforms{Build: build, Host: host, Target: target}
			check(t, DepsBuildBuild, in, Platforms{build, build, build})
			check(t, DepsBuildHost, in, Platforms{build, build, host})
			check(t, DepsBuildTarget, in, Platforms{build, build, target})
			check(t, DepsHostHost, in, Platforms{build, host, host})
			check(t, DepsHostTarget, in, Platforms{build, host, target})
			check(t, DepsTargetTarget, in, Platforms{build, target, target})
		})

		t.Run("universal host and target", func(t *ftt.Test) {
			in := Platforms{Build: build, Host: uni, Target: uni}
			check(t, DepsBuildBuild, in, Platforms{build, build, build})
			check(t, DepsBuildHost, in, Platforms{build, build, build})
			check(t, DepsBuildTarget, in, Platforms{build, build, build})
			check(t, DepsHostHost, in, Platforms{build, uni, uni})
			check(t, DepsHostTarget, in, Platforms{build, uni, uni})
			check(t, DepsTargetTarget, in, Platforms{build, uni, uni})
		})

		t.Run("universal target with concrete host", func(t *ftt.Test) {
			in := Platforms{Build: build, Host: host, Target: uni}
			check(t, DepsBuildTarget, in, Platforms{build, build, build})
			check(t, DepsHostTarget, in, Platforms{build, host, host})
		})

		t.Run("unknown dependency type", func(t *ftt.Test) {
			dep := &Dependency{Generator: &capturePlatformsGenerator{}, Type: DepsUnknown}
			_, err := dep.Generate(ctx, Platforms{Build: build, Host: host, Target: target})
			assert.That(t, err, should.ErrLike(ErrUnknowDependencyType))
		})
	})
}
