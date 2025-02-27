// Copyright 2019 The LUCI Authors.
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

package pbutil

import (
	"fmt"
	"regexp"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/validate"
)

// fieldDoesNotMatch returns the string of unspecified error with the field name.
func fieldUnspecified(fieldName string) string {
	return fmt.Sprintf("%s: %s", fieldName, validate.Unspecified())
}

// fieldDoesNotMatch returns the string of doesNotMatch error with the field name.
func fieldDoesNotMatch(fieldName string, re *regexp.Regexp) string {
	return fmt.Sprintf("%s: %s", fieldName, validate.DoesNotMatchReErr(re))
}

func TestTestResultName(t *testing.T) {
	t.Parallel()

	ftt.Run("ParseTestResultName", t, func(t *ftt.Test) {
		t.Run("Parse", func(t *ftt.Test) {
			invID, testID, resultID, err := ParseTestResultName(
				"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invID, should.Equal("a"))
			assert.Loosely(t, testID, should.Equal("ninja://chrome/test:foo_tests/BarTest.DoBaz"))
			assert.Loosely(t, resultID, should.Equal("result5"))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			t.Run(`has slashes`, func(t *ftt.Test) {
				_, _, _, err := ParseTestResultName(
					"invocations/inv/tests/ninja://test/results/result1")
				assert.Loosely(t, err, should.ErrLike("does not match pattern"))
			})

			t.Run(`bad unescape`, func(t *ftt.Test) {
				_, _, _, err := ParseTestResultName(
					"invocations/a/tests/bad_hex_%gg/results/result1")
				assert.Loosely(t, err, should.ErrLike("test id"))
			})

			t.Run(`unescaped unprintable`, func(t *ftt.Test) {
				_, _, _, err := ParseTestResultName(
					"invocations/a/tests/unprintable_%07/results/result1")
				assert.Loosely(t, err, should.ErrLike("non-printable rune"))
			})
		})

		t.Run("Format", func(t *ftt.Test) {
			assert.Loosely(t, TestResultName("a", "ninja://chrome/test:foo_tests/BarTest.DoBaz", "result5"),
				should.Equal(
					"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5"))
		})
	})
}
