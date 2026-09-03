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
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestAntsHelpers(t *testing.T) {
	ftt.Run("AnTS URL and ID parsing", t, func(t *ftt.Test) {
		t.Run("IsAntsTestResultID", func(t *ftt.Test) {
			assert.Loosely(t, IsAntsTestResultID("TR29830335282615697"), should.BeTrue)
			assert.Loosely(t, IsAntsTestResultID("TR1"), should.BeTrue)
			assert.Loosely(t, IsAntsTestResultID("tr123"), should.BeTrue)
			assert.Loosely(t, IsAntsTestResultID("TR"), should.BeFalse)
			assert.Loosely(t, IsAntsTestResultID("123"), should.BeFalse)
			assert.Loosely(t, IsAntsTestResultID("invocations/TR123"), should.BeFalse)
		})

		t.Run("IsAntsInvocationID", func(t *ftt.Test) {
			assert.Loosely(t, IsAntsInvocationID("I77100010600769898"), should.BeTrue)
			assert.Loosely(t, IsAntsInvocationID("I1"), should.BeTrue)
			assert.Loosely(t, IsAntsInvocationID("ants-I77100010600769898"), should.BeTrue)
			assert.Loosely(t, IsAntsInvocationID("ants-i77100010600769898"), should.BeTrue)
			assert.Loosely(t, IsAntsInvocationID("I"), should.BeFalse)
		})

		t.Run("IsAntsWorkUnitID", func(t *ftt.Test) {
			assert.Loosely(t, IsAntsWorkUnitID("WU17100269020689387"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("WU1"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("ants-WU123"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("ants-wu83500269020198004"), should.BeTrue)
			assert.Loosely(t, IsAntsWorkUnitID("WU"), should.BeFalse)
		})

		t.Run("IsAntsURL and ExtractAntsURLComponents", func(t *ftt.Test) {
			url1 := "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898/test/TR29830335282615697/"
			assert.Loosely(t, IsAntsURL(url1), should.BeTrue)
			inv1, tr1 := ExtractAntsURLComponents(url1)
			assert.Loosely(t, inv1, should.Equal("I77100010600769898"))
			assert.Loosely(t, tr1, should.Equal("TR29830335282615697"))

			url2 := "https://android-build.corp.google.com/test_investigate/test/TR13830335277435395"
			assert.Loosely(t, IsAntsURL(url2), should.BeTrue)
			inv2, tr2 := ExtractAntsURLComponents(url2)
			assert.Loosely(t, inv2, should.Equal(""))
			assert.Loosely(t, tr2, should.Equal("TR13830335277435395"))

			url3 := "https://android-build.corp.google.com/test_investigate/invocation/I77100010600769898"
			assert.Loosely(t, IsAntsURL(url3), should.BeTrue)
			inv3, tr3 := ExtractAntsURLComponents(url3)
			assert.Loosely(t, inv3, should.Equal("I77100010600769898"))
			assert.Loosely(t, tr3, should.Equal(""))

			url4 := "https://android-build.googleplex.com/builds/tests/view?invocationId=I30800010616707848"
			assert.Loosely(t, IsAntsURL(url4), should.BeTrue)
			inv4, tr4 := ExtractAntsURLComponents(url4)
			assert.Loosely(t, inv4, should.Equal("I30800010616707848"))
			assert.Loosely(t, tr4, should.Equal(""))

			url5 := "https://android-build.corp.google.com/builds/tests/view?invocation_id=I30800010616707848&testResultId=TR29830335282615697"
			assert.Loosely(t, IsAntsURL(url5), should.BeTrue)
			inv5, tr5 := ExtractAntsURLComponents(url5)
			assert.Loosely(t, inv5, should.Equal("I30800010616707848"))
			assert.Loosely(t, tr5, should.Equal("TR29830335282615697"))

			nonAnts := "https://ci.chromium.org/ui/test-investigate/invocations/build-123"
			assert.Loosely(t, IsAntsURL(nonAnts), should.BeFalse)
		})

		t.Run("NormalizeWorkUnit", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeWorkUnit("WU17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("ants-WU17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("ants-wu17100269020689387"), should.Equal("ants-wu17100269020689387"))
			assert.Loosely(t, NormalizeWorkUnit("workUnits/WU123"), should.Equal("ants-wu123"))
			assert.Loosely(t, NormalizeWorkUnit("root"), should.Equal("root"))
		})

		t.Run("NormalizeInvocation with AnTS IDs", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeInvocation("I77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("ants-I77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("ants-i77100010600769898"), should.Equal("ants-i77100010600769898"))
			assert.Loosely(t, NormalizeInvocation("invocations/I77100010600769898"), should.Equal("ants-i77100010600769898"))
		})

		t.Run("ParseAntsShowTestOutput", func(t *ftt.Test) {
			methodOutput := `Fetching details for test result: TR13830335277435395...
Test Result ID: TR13830335277435395
Test Case: CellBroadcastServiceTests#com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest.testResetAreaInfoWithDefaultSubChanged
Status: pass
Work Unit ID: WU17100269020689387
Invocation ID: I77100010600769898
Run Number: 0
Attempt Number: 1
Test Identifier ID: a1a9ab35503045aa069b820825a597e3`

			info, err := ParseAntsShowTestOutput(methodOutput)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info.TestResultID, should.Equal("TR13830335277435395"))
			assert.Loosely(t, info.TestCase, should.Equal("CellBroadcastServiceTests#com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest.testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, info.ModuleName, should.Equal("CellBroadcastServiceTests"))
			assert.Loosely(t, info.ClassName, should.Equal("com.android.cellbroadcastservice.tests.GsmCellBroadcastHandlerTest"))
			assert.Loosely(t, info.MethodName, should.Equal("testResetAreaInfoWithDefaultSubChanged"))
			assert.Loosely(t, info.Status, should.Equal("pass"))
			assert.Loosely(t, info.WorkUnitID, should.Equal("WU17100269020689387"))
			assert.Loosely(t, info.InvocationID, should.Equal("I77100010600769898"))
			assert.Loosely(t, info.RunNumber, should.Equal(0))
			assert.Loosely(t, info.AttemptNumber, should.Equal(1))
			assert.Loosely(t, info.IsModuleError, should.BeFalse)

			moduleErrorOutput := `Fetching details for test result: TR29830335282615697...
Test Result ID: TR29830335282615697
Test Case: CellBroadcastReceiverMTS#.
Status: testError
Work Unit ID:
Invocation ID: I77100010600769898
Run Number: 0
Attempt Number: 0
Test Identifier ID: 6fd7643ef8e5a458862c7e4a68e88e2c`

			info2, err := ParseAntsShowTestOutput(moduleErrorOutput)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info2.TestResultID, should.Equal("TR29830335282615697"))
			assert.Loosely(t, info2.ModuleName, should.Equal("CellBroadcastReceiverMTS"))
			assert.Loosely(t, info2.ClassName, should.Equal(""))
			assert.Loosely(t, info2.MethodName, should.Equal(""))
			assert.Loosely(t, info2.IsModuleError, should.BeTrue)
			assert.Loosely(t, info2.Status, should.Equal("testError"))
			assert.Loosely(t, info2.InvocationID, should.Equal("I77100010600769898"))

			// Test JSON format parsing
			jsonOutput := `{"test_result_id":"TR55555","test_case":"MyModule#com.example.Test.method1","status":"pass","invocation_id":"I99999"}`
			info3, err := ParseAntsShowTestOutput(jsonOutput)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info3.TestResultID, should.Equal("TR55555"))
			assert.Loosely(t, info3.ModuleName, should.Equal("MyModule"))
			assert.Loosely(t, info3.ClassName, should.Equal("com.example.Test"))
			assert.Loosely(t, info3.MethodName, should.Equal("method1"))
			assert.Loosely(t, info3.InvocationID, should.Equal("I99999"))
		})

		t.Run("FindAntsCLI with ANTS_CLI_PATH override", func(t *ftt.Test) {
			tempDir := t.TempDir()
			fakeBin := filepath.Join(tempDir, "ants_cli")
			_ = os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755)

			t.Setenv("ANTS_CLI_PATH", fakeBin)
			p, err := FindAntsCLI()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, p, should.Equal(fakeBin))
		})
	})
}
