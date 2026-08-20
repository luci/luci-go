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
		tempDir := t.TempDir()
		SetTestCacheDir(tempDir)

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

		t.Run("ParseTestResultTargetArgs", func(t *ftt.Test) {
			t.Run("direct URL", func(t *ftt.Test) {
				res, err := ParseTestResultTargetArgs([]string{"invocations/build-1/tests/t1/results/r1"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("invocations/build-1/tests/t1/results/r1"))
			})

			t.Run("decomposed IDs", func(t *ftt.Test) {
				res, err := ParseTestResultTargetArgs([]string{"build-1", "t1", "r1"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("invocations/build-1/tests/t1/results/r1"))
			})

			t.Run("cache override with -", func(t *ftt.Test) {
				RecordTestResult("invocations/build-1/tests/t1/results/r1", "build-1", "t1")
				res, err := ParseTestResultTargetArgs([]string{"-", "r2"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("invocations/build-1/tests/t1/results/r2"))
			})

			t.Run("single short non-url fails without -", func(t *ftt.Test) {
				_, err := ParseTestResultTargetArgs([]string{"01a5d9e8-00081"})
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("is not a full test result resource name or URL"))
			})
		})

		t.Run("ParseWorkUnitTargetArgs", func(t *ftt.Test) {
			t.Run("direct URL", func(t *ftt.Test) {
				res, err := ParseWorkUnitTargetArgs([]string{"rootInvocations/ants-1/workUnits/wu-1"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("rootInvocations/ants-1/workUnits/wu-1"))
			})

			t.Run("decomposed IDs", func(t *ftt.Test) {
				res, err := ParseWorkUnitTargetArgs([]string{"ants-1", "wu-1"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("rootInvocations/ants-1/workUnits/wu-1"))
			})

			t.Run("cache with -", func(t *ftt.Test) {
				RecordWorkUnit("rootInvocations/ants-1/workUnits/wu-1", "ants-1")
				res, err := ParseWorkUnitTargetArgs([]string{"-"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("rootInvocations/ants-1/workUnits/wu-1"))
			})
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

		t.Run("ExtractWorkUnitComponents", func(t *ftt.Test) {
			rootInv, wuID := ExtractWorkUnitComponents("rootInvocations/ants-i123/workUnits/wu-1")
			assert.Loosely(t, rootInv, should.Equal("ants-i123"))
			assert.Loosely(t, wuID, should.Equal("wu-1"))
		})
	})
}
