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

type staticGenerator struct {
	action *core.Action
}

func (g *staticGenerator) Generate(context.Context, Platforms) (*core.Action, error) {
	return g.action, nil
}

func TestRef(t *testing.T) {
	ftt.Run("Test Ref", t, func(t *ftt.Test) {
		ctx := context.Background()
		plats := Platforms{}

		t.Run("aliases name and inherits runtime deps and untrusted", func(t *ftt.Test) {
			rdep := &core.Action{
				Name: "runtime_dep",
				Spec: &core.Action_Command{Command: &core.ActionCommand{Args: []string{"rd"}}},
			}
			target := &core.Action{
				Name: "runc_url_deadbeef",
				Metadata: &core.Action_Metadata{
					RuntimeDeps: []*core.Action{rdep},
					Untrusted:   true,
					Cipd:        &core.Action_Metadata_CIPD{Name: "crowbar/runc/fetch"},
				},
				Spec: &core.Action_Command{Command: &core.ActionCommand{Args: []string{"fetch"}}},
			}

			g := &Ref{
				Name: "runc",
				Metadata: &core.Action_Metadata{
					Cipd: &core.Action_Metadata_CIPD{Name: "crowbar/runc/linux-amd64"},
				},
				Target: &staticGenerator{action: target},
			}

			a, err := g.Generate(ctx, plats)
			assert.NoErr(t, err)
			assert.Loosely(t, a.Name, should.Equal("runc"))
			assert.Loosely(t, a.GetRef(), should.NotBeNil)
			assert.Loosely(t, a.Deps, should.Match([]*core.Action{target}))
			assert.Loosely(t, a.Metadata.GetCipd().GetName(), should.Equal("crowbar/runc/linux-amd64"))
			assert.Loosely(t, a.Metadata.GetUntrusted(), should.BeTrue)
			assert.Loosely(t, a.Metadata.GetRuntimeDeps(), should.Match([]*core.Action{rdep}))
			// Target metadata is not mutated.
			assert.Loosely(t, target.Metadata.GetCipd().GetName(), should.Equal("crowbar/runc/fetch"))
			assert.Loosely(t, g.Metadata.GetUntrusted(), should.BeFalse)
		})

		t.Run("defaults name to target name", func(t *ftt.Test) {
			target := &core.Action{
				Name: "orig",
				Spec: &core.Action_Command{Command: &core.ActionCommand{Args: []string{"cmd"}}},
			}
			a, err := (&Ref{Target: &staticGenerator{action: target}}).Generate(ctx, plats)
			assert.NoErr(t, err)
			assert.Loosely(t, a.Name, should.Equal("orig"))
			assert.Loosely(t, a.Metadata, should.BeNil)
		})

		t.Run("errors on nil target", func(t *ftt.Test) {
			_, err := (&Ref{Name: "empty"}).Generate(ctx, plats)
			assert.Loosely(t, err, should.ErrLike("target must not be nil"))
		})
	})
}
