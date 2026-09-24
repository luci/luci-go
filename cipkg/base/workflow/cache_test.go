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

package workflow

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/testutils"
)

func TestRelocatableID(t *testing.T) {
	ftt.Run("Test RelocatableID", t, func(t *ftt.Test) {
		ap := actions.NewActionProcessor()
		pm1 := testutils.NewMockPackageManage("/storage/one")
		pm2 := testutils.NewMockPackageManage("/storage/two")

		makeAction := func() *core.Action {
			return &core.Action{
				Name: "pkg",
				Deps: []*core.Action{
					{Name: "dep", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Command{
					Command: &core.ActionCommand{
						Args: []string{"bin", actions.DepRef("dep")},
					},
				},
			}
		}

		pkg1, err := ap.Process("", pm1, makeAction())
		assert.Loosely(t, err, should.BeNil)
		pkg2, err := ap.Process("", pm2, makeAction())
		assert.Loosely(t, err, should.BeNil)

		// DerivationID differs because local storage paths differ.
		assert.Loosely(t, pkg1.DerivationID, should.NotEqual(pkg2.DerivationID))
		assert.Loosely(t, pkg1.ActionID, should.Equal(pkg2.ActionID))

		// RelocatableID is reproducible across different storage paths.
		id1, err := RelocatableCacheID("", ap, pkg1.Action)
		assert.Loosely(t, err, should.BeNil)
		id2, err := RelocatableCacheID("", ap, pkg2.Action)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, id1, should.NotBeEmpty)
		assert.Loosely(t, id1, should.Equal(id2))
		assert.Loosely(t, id1, should.NotEqual(pkg1.DerivationID))
		assert.Loosely(t, id2, should.NotEqual(pkg2.DerivationID))
	})
}
