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

package base

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTargets(t *testing.T) {
	ftt.Run("Targets parsing and formatting", t, func(t *ftt.Test) {
		t.Run("TrimResourceURL", func(t *ftt.Test) {
			assert.Loosely(t, TrimResourceURL("https://ci.chromium.org/ui/test-investigate/invocations/build-123/tests/test1?result=r1#top"), should.Equal("invocations/build-123/tests/test1"))
			assert.Loosely(t, TrimResourceURL("https://ci.chromium.org/ui/test-investigate/rootInvocations/ants-123/workUnits/wu-456"), should.Equal("rootInvocations/ants-123/workUnits/wu-456"))
		})

		t.Run("FormatTestResultResourceName", func(t *ftt.Test) {
			assert.Loosely(t, FormatTestResultResourceName("build-123", "test/one", "res1"), should.Equal("invocations/build-123/tests/test%2Fone/results/res1"))
			assert.Loosely(t, FormatTestResultResourceName("8673802696052024673", "test/one", "res1"), should.Equal("invocations/build-8673802696052024673/tests/test%2Fone/results/res1"))
		})

		t.Run("FormatWorkUnitResourceName", func(t *ftt.Test) {
			assert.Loosely(t, FormatWorkUnitResourceName("ants-123", "wu-456"), should.Equal("rootInvocations/ants-123/workUnits/wu-456"))
			assert.Loosely(t, FormatWorkUnitResourceName("8673802696052024673", "wu-456"), should.Equal("rootInvocations/build-8673802696052024673/workUnits/wu-456"))
		})

		t.Run("ExtractTestResultComponents", func(t *ftt.Test) {
			inv, testID, resID := ExtractTestResultComponents("rootInvocations/ants-i123/workUnits/wu-1/tests/:MTS!junit:Test#case/results/res-01")
			assert.Loosely(t, inv, should.Equal("ants-i123"))
			assert.Loosely(t, testID, should.Equal(":MTS!junit:Test#case"))
			assert.Loosely(t, resID, should.Equal("res-01"))

			inv2, testID2, resID2 := ExtractTestResultComponents("invocations/build-123/tests/test_foo/results/0")
			assert.Loosely(t, inv2, should.Equal("build-123"))
			assert.Loosely(t, testID2, should.Equal("test_foo"))
			assert.Loosely(t, resID2, should.Equal("0"))
		})

		t.Run("NormalizeInvocation", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeInvocation("b8676886509240051393"), should.Equal("build-8676886509240051393"))
			assert.Loosely(t, NormalizeInvocation("8676886509240051393"), should.Equal("build-8676886509240051393"))
			assert.Loosely(t, NormalizeInvocation("I77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("ants-I77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("invocations/build-123"), should.Equal("build-123"))
			assert.Loosely(t, NormalizeInvocation("rootInvocations/build-123"), should.Equal("build-123"))
		})

		t.Run("NormalizeWorkUnit", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeWorkUnit("WU17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("wu17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("workUnits/wu-1"), should.Equal("wu-1"))
		})
	})
}
