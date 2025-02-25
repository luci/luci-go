// Copyright 2021 The LUCI Authors.
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

package state

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

func TestUpgradeIfNecessary(t *testing.T) {
	t.Parallel()

	ftt.Run("UpgradeIfNecessary works", t, func(t *ftt.Test) {
		s0 := &State{PB: &prjpb.PState{
			Pcls: []*prjpb.PCL{
				{Clid: 1},
				{Clid: 2},
			},
		}}

		t.Run("not necessary", func(t *ftt.Test) {
			pb := backupPB(s0)
			s1 := s0.UpgradeIfNecessary()
			assert.Loosely(t, s1, should.Equal(s0))
			assert.Loosely(t, s0.PB, should.Match(pb))
		})

		t.Run("necessary", func(t *ftt.Test) {
			// Add test when needed.
		})
	})
}
