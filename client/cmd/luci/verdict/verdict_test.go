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

package verdict

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestParseVerdictName(t *testing.T) {
	t.Parallel()

	ftt.Run(`ParseVerdictName`, t, func(t *ftt.Test) {
		t.Run(`structured UI URL with domain and query params`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/test-investigate/invocations/build-8676886509240051393/modules/%2F%2Fchrome%3Achrome_private_code_test/schemes/single/variants/b7de9035241e76cc/cases/*fixture?artifact=summary_node"
			invName, variantHash, testIDRegexp, matchFunc, err := ParseVerdictName(url)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invName, should.Equal("invocations/build-8676886509240051393"))
			assert.Loosely(t, variantHash, should.Equal("b7de9035241e76cc"))
			assert.Loosely(t, testIDRegexp, should.Equal(`.*\*fixture`))

			tr := &pb.TestResult{
				VariantHash: "b7de9035241e76cc",
				TestIdStructured: &pb.TestIdentifier{
					ModuleName: "//chrome:chrome_private_code_test",
					CaseName:   "*fixture",
				},
			}
			assert.Loosely(t, matchFunc(tr), should.BeTrue)

			trWrongVariant := &pb.TestResult{
				VariantHash: "otherhash",
				TestIdStructured: &pb.TestIdentifier{
					ModuleName: "//chrome:chrome_private_code_test",
					CaseName:   "*fixture",
				},
			}
			assert.Loosely(t, matchFunc(trWrongVariant), should.BeFalse)
		})

		t.Run(`flat format`, func(t *ftt.Test) {
			name := "invocations/build-123/tests/ninja%3A%2F%2Fchrome%2Ftest%3Abrowser_tests%2FMyTest.Case/variants/hash123"
			invName, variantHash, testIDRegexp, matchFunc, err := ParseVerdictName(name)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invName, should.Equal("invocations/build-123"))
			assert.Loosely(t, variantHash, should.Equal("hash123"))
			assert.Loosely(t, testIDRegexp, should.Equal(`ninja://chrome/test:browser_tests/MyTest\.Case`))

			tr := &pb.TestResult{
				VariantHash: "hash123",
				TestId:      "ninja://chrome/test:browser_tests/MyTest.Case",
			}
			assert.Loosely(t, matchFunc(tr), should.BeTrue)
		})

		t.Run(`missing variants`, func(t *ftt.Test) {
			_, _, _, _, err := ParseVerdictName("invocations/build-123/tests/mytest")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestCmdVerdict(t *testing.T) {
	t.Parallel()

	ftt.Run(`Cmd`, t, func(t *ftt.Test) {
		cmd := Cmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("verdict <subcommand>"))

		getCmd := GetCmd(nil)
		assert.Loosely(t, getCmd, should.NotBeNil)
		assert.Loosely(t, getCmd.UsageLine, should.Equal("get <name>"))
	})
}
