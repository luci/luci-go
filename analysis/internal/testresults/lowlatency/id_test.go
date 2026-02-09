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

package lowlatency

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRootInvocationID(t *testing.T) {
	ftt.Run("RootInvocationID", t, func(t *ftt.Test) {
		t.Run("RowID", func(t *ftt.Test) {
			assert.Loosely(t, RootInvocationID{Value: "a"}.RowID(), should.Equal("root:a"))
			assert.Loosely(t, RootInvocationID{Value: "a", IsLegacy: true}.RowID(), should.Equal("legacy:a"))
		})

		t.Run("FromRowID", func(t *ftt.Test) {
			assert.Loosely(t, RootInvocationIDFromRowID("root:a"), should.Match(RootInvocationID{Value: "a"}))
			assert.Loosely(t, RootInvocationIDFromRowID("legacy:a"), should.Match(RootInvocationID{Value: "a", IsLegacy: true}))
			assert.Loosely(t, RootInvocationIDFromRowID("a"), should.Match(RootInvocationID{Value: "a", IsLegacy: true}))
		})
	})
}

func TestWorkUnitID(t *testing.T) {
	ftt.Run("WorkUnitID", t, func(t *ftt.Test) {
		t.Run("RowID", func(t *ftt.Test) {
			assert.Loosely(t, WorkUnitID{Value: "a"}.RowID(), should.Equal("wu:a"))
			assert.Loosely(t, WorkUnitID{Value: "a", IsLegacy: true}.RowID(), should.Equal("legacy:a"))
		})

		t.Run("FromRowID", func(t *ftt.Test) {
			assert.Loosely(t, WorkUnitIDFromRowID("wu:a"), should.Match(WorkUnitID{Value: "a"}))
			assert.Loosely(t, WorkUnitIDFromRowID("legacy:a"), should.Match(WorkUnitID{Value: "a", IsLegacy: true}))
			assert.Loosely(t, WorkUnitIDFromRowID("a"), should.Match(WorkUnitID{Value: "a", IsLegacy: true}))
		})
	})
}
