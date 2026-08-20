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
	"os"
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

		t.Run(`workflow sequence: verdict -> test-result -> work-unit -> test-result artifact`, func(t *ftt.Test) {
			inv := "ants-i77100010600769898"
			verdictURL := "https://ci.chromium.org/ui/test-investigate/invocations/" + inv + "/modules/CellBroadcastReceiverMTS/..."
			RecordVerdict(inv, verdictURL)

			// Step 2: test-result get
			trName := "rootInvocations/" + inv + "/workUnits/ants-wu83500269020198004:u-3ef7aa48-a4fd-41aa-b665-be69e04b07f2/tests/:CellBroadcastReceiverMTS!junit:Test#test1/results/0efc0e5a-00528"
			RecordTestResult(trName, "", "")

			// Step 3: work-unit get
			wuName := "rootInvocations/" + inv + "/workUnits/ants-wu83500269020198004"
			RecordWorkUnit(wuName, "")

			cd, err := LoadCache()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cd.Invocation, should.Equal(inv))
			assert.Loosely(t, cd.Verdict, should.Equal(verdictURL))
			assert.Loosely(t, cd.TestResult, should.Equal(trName))
			assert.Loosely(t, cd.TestID, should.Equal(":CellBroadcastReceiverMTS!junit:Test#test1"))
			assert.Loosely(t, cd.ResultID, should.Equal("0efc0e5a-00528"))
			assert.Loosely(t, cd.WorkUnit, should.Equal(wuName))
			assert.Loosely(t, cd.WorkUnitID, should.Equal("ants-wu83500269020198004"))

			// Step 4: resolve test result target using '-'
			target, err := ParseTestResultTargetArgs([]string{"-"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, target, should.Equal(trName))
		})

		t.Run(`session isolation for parallel agents`, func(t *ftt.Test) {
			// Session A (e.g. Agent 1 on terminal /dev/pts/1)
			SetTestSessionID("pts_1")
			RecordTestResult("invocations/build-100/tests/test_a/results/0", "build-100", "test_a")
			RecordArtifact("log_a")

			cdA, err := LoadCache()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cdA.Invocation, should.Equal("build-100"))
			assert.Loosely(t, cdA.TestID, should.Equal("test_a"))
			assert.Loosely(t, cdA.Artifact, should.Equal("log_a"))

			// Session B (e.g. Agent 2 on terminal /dev/pts/2)
			SetTestSessionID("pts_2")
			RecordTestResult("invocations/build-200/tests/test_b/results/1", "build-200", "test_b")
			RecordArtifact("log_b")

			cdB, err := LoadCache()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cdB.Invocation, should.Equal("build-200"))
			assert.Loosely(t, cdB.TestID, should.Equal("test_b"))
			assert.Loosely(t, cdB.Artifact, should.Equal("log_b"))

			// Switch back to Session A: should not be stomped by Session B!
			SetTestSessionID("pts_1")
			cdA2, err := LoadCache()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cdA2.Invocation, should.Equal("build-100"))
			assert.Loosely(t, cdA2.TestID, should.Equal("test_a"))
			assert.Loosely(t, cdA2.Artifact, should.Equal("log_a"))

			// Clean up test session
			SetTestSessionID("")
		})

		t.Run(`sanitizeSessionID`, func(t *ftt.Test) {
			assert.Loosely(t, sanitizeSessionID("/dev/pts/1"), should.Equal("pts_1"))
			assert.Loosely(t, sanitizeSessionID("/dev/tty2"), should.Equal("tty2"))
			assert.Loosely(t, sanitizeSessionID("agent:123.456"), should.Equal("agent_123_456"))
			assert.Loosely(t, sanitizeSessionID("bc3589ff-6ae0-4168-be56-7e0c6ba446fa"), should.Equal("bc3589ff-6ae0-4168-be56-7e0c6ba446fa"))
			assert.Loosely(t, sanitizeSessionID(""), should.Equal("default"))
		})

		t.Run(`agent environment variable session detection`, func(t *ftt.Test) {
			SetTestSessionID("")
			t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "conv-xyz-123")
			assert.Loosely(t, GetSessionID(), should.Equal("conv-xyz-123"))

			t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "")
			t.Setenv("INVOKER_INFO_SESSION_ID", "invoker-456")
			assert.Loosely(t, GetSessionID(), should.Equal("invoker-456"))
		})

		t.Run(`fallback to recent cache file if session-specific cache is empty`, func(t *ftt.Test) {
			// Write cache under session A (e.g. background task with conv-xyz)
			SetTestSessionID("conv-xyz")
			RecordTestResult("invocations/build-300/tests/test_c/results/0", "build-300", "test_c")

			// Load from session B (e.g. detached foreground task with no session set)
			SetTestSessionID("detached-session")
			cd, err := LoadCache()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cd.Invocation, should.Equal("build-300"))
			assert.Loosely(t, cd.TestID, should.Equal("test_c"))

			SetTestSessionID("")
		})

		t.Run(`fallback to recent cache file in os.TempDir() matches UID prefix`, func(t *ftt.Test) {
			tempDir := t.TempDir()
			// Override cacheDir to tempDir, but test the prefix logic when getCacheDir() returns os.TempDir()
			SetTestCacheDir(os.TempDir())
			defer SetTestCacheDir(tempDir)

			SetTestSessionID("tempdir-session-1")
			RecordTestResult("invocations/build-400/tests/test_d/results/0", "build-400", "test_d")

			SetTestSessionID("tempdir-session-2")
			cd, err := LoadCache()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cd.Invocation, should.Equal("build-400"))
			assert.Loosely(t, cd.TestID, should.Equal("test_d"))

			SetTestSessionID("")
		})
	})
}
