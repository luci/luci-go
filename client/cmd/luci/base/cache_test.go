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

func TestCache(t *testing.T) {
	ftt.Run(`Cache operations and invalidations`, t, func(t *ftt.Test) {
		tempDir := t.TempDir()
		SetTestCacheDir(tempDir)

		t.Run(`record and retrieve test result`, func(t *ftt.Test) {
			trName := "invocations/build-100/tests/ninja:%2F%2Fchrome%2Ftest:browser_tests/results/0"
			RecordTestResult(trName, "", "")

			cd, err := LoadCache()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cd.Invocation, should.Equal("build-100"))
			assert.Loosely(t, cd.TestID, should.Equal("ninja://chrome/test:browser_tests"))
			assert.Loosely(t, cd.ResultID, should.Equal("0"))
			assert.Loosely(t, cd.TestResult, should.Equal(trName))
		})

		t.Run(`record work unit`, func(t *ftt.Test) {
			wuName := "rootInvocations/build-100/workUnits/run-tests"
			RecordWorkUnit(wuName, "")
			RecordArtifact("stdout")

			cd, err := LoadCache()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cd.Invocation, should.Equal("build-100"))
			assert.Loosely(t, cd.WorkUnitID, should.Equal("run-tests"))
			assert.Loosely(t, cd.WorkUnit, should.Equal(wuName))
			assert.Loosely(t, cd.Artifact, should.Equal("stdout"))
		})

		t.Run(`changing invocation clears work unit, test result and artifact`, func(t *ftt.Test) {
			// Setup inv1 with work unit and artifact
			RecordWorkUnit("rootInvocations/build-100/workUnits/wu1", "")
			RecordArtifact("log")

			cd, _ := LoadCache()
			assert.Loosely(t, cd.Invocation, should.Equal("build-100"))
			assert.Loosely(t, cd.WorkUnitID, should.Equal("wu1"))
			assert.Loosely(t, cd.Artifact, should.Equal("log"))

			// Now user calls test-result get with inv2
			RecordTestResult("invocations/build-200/tests/my_test/results/1", "build-200", "my_test")

			cd2, _ := LoadCache()
			assert.Loosely(t, cd2.Invocation, should.Equal("build-200"))
			assert.Loosely(t, cd2.TestID, should.Equal("my_test"))
			assert.Loosely(t, cd2.ResultID, should.Equal("1"))
			// WorkUnit and previous artifact MUST be cleared!
			assert.Loosely(t, cd2.WorkUnitID, should.BeEmpty)
			assert.Loosely(t, cd2.WorkUnit, should.BeEmpty)
			assert.Loosely(t, cd2.Artifact, should.BeEmpty)
		})

		t.Run(`changing test ID within same invocation clears result and artifact`, func(t *ftt.Test) {
			RecordTestResult("invocations/build-200/tests/test_a/results/0", "build-200", "test_a")
			RecordArtifact("screenshot")

			_ = SaveCache(func(cd *CacheData) {
				cd.SetTestID("test_b")
			})

			cd, _ := LoadCache()
			assert.Loosely(t, cd.Invocation, should.Equal("build-200"))
			assert.Loosely(t, cd.TestID, should.Equal("test_b"))
			assert.Loosely(t, cd.ResultID, should.BeEmpty)
			assert.Loosely(t, cd.TestResult, should.BeEmpty)
			assert.Loosely(t, cd.Artifact, should.BeEmpty)
		})

		t.Run(`record verdict strictly records user arguments`, func(t *ftt.Test) {
			verdictURL := "https://ci.chromium.org/ui/test-investigate/invocations/build-8673802696052024673/..."
			RecordVerdict("build-8673802696052024673", verdictURL)

			cd, err := LoadCache()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cd.Invocation, should.Equal("build-8673802696052024673"))
			assert.Loosely(t, cd.Verdict, should.Equal(verdictURL))
			assert.Loosely(t, cd.TestID, should.BeEmpty)
			assert.Loosely(t, cd.ResultID, should.BeEmpty)
			assert.Loosely(t, cd.TestResult, should.BeEmpty)
		})
	})
}
