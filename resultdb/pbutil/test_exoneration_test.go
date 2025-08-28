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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseLegacyExonerationName(t *testing.T) {
	t.Parallel()
	ftt.Run("ParseLegacyTestExonerationName", t, func(t *ftt.Test) {
		t.Run("Parse", func(t *ftt.Test) {
			inv, testID, ex, err := ParseLegacyTestExonerationName("invocations/a/tests/b%2Fc/exonerations/1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Equal("a"))
			assert.Loosely(t, testID, should.Equal("b/c"))
			assert.Loosely(t, ex, should.Equal("1"))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			_, _, _, err := ParseLegacyTestExonerationName("invocations/a/tests/b/c/exonerations/1")
			assert.Loosely(t, err, should.ErrLike(`does not match`))
		})

		t.Run("Format", func(t *ftt.Test) {
			assert.Loosely(t, LegacyTestExonerationName("a", "b/c", "d"), should.Equal("invocations/a/tests/b%2Fc/exonerations/d"))
		})
	})
}

func TestTestExonerationName(t *testing.T) {
	t.Parallel()

	ftt.Run("ParseTestExonerationName", t, func(t *ftt.Test) {
		t.Run("Parse", func(t *ftt.Test) {
			parts, err := ParseTestExonerationName(
				"rootInvocations/a/workUnits/b/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/exonerations/ab23efabcdef:d:ab23efabcdef")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parts.RootInvocationID, should.Equal("a"))
			assert.Loosely(t, parts.WorkUnitID, should.Equal("b"))
			assert.Loosely(t, parts.TestID, should.Equal("ninja://chrome/test:foo_tests/BarTest.DoBaz"))
			assert.Loosely(t, parts.ExonerationID, should.Equal("ab23efabcdef:d:ab23efabcdef"))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			t.Run(`unescaped test ID`, func(t *ftt.Test) {
				_, err := ParseTestExonerationName(
					"rootInvocations/inv/workUnits/wu/tests/ninja://test/exonerations/some-id")
				assert.Loosely(t, err, should.ErrLike("does not match pattern"))
			})
			t.Run(`bad escaping`, func(t *ftt.Test) {
				_, err := ParseTestExonerationName(
					"rootInvocations/a/workUnits/b/tests/bad_hex_%gg/exonerations/some-id")
				assert.Loosely(t, err, should.ErrLike("test ID"))
			})
			t.Run(`unescaped unprintable`, func(t *ftt.Test) {
				_, err := ParseTestExonerationName(
					"rootInvocations/a/workUnits/b/tests/unprintable_%07/exonerations/some-id")
				assert.Loosely(t, err, should.ErrLike("non-printable rune"))
			})
			t.Run(`bad work unit name`, func(t *ftt.Test) {
				_, err := ParseTestExonerationName(
					"rootInvocations/a/workUnits/" + strings.Repeat("b", workUnitIDMaxLength+1) + "/tests/test-id/exonerations/some-id")
				assert.Loosely(t, err, should.ErrLike("work unit ID"))
			})
			t.Run(`bad root invocation name`, func(t *ftt.Test) {
				_, err := ParseTestExonerationName(
					"rootInvocations/" + strings.Repeat("a", rootInvocationMaxLength+1) + "/workUnits/b/tests/test-id/exonerations/some-id")
				assert.Loosely(t, err, should.ErrLike("root invocation ID"))
			})
			t.Run(`bad exoneration ID`, func(t *ftt.Test) {
				_, err := ParseTestExonerationName(
					"rootInvocations/a/workUnits/b/tests/some-test/exonerations/!invalid")
				assert.Loosely(t, err, should.ErrLike("exoneration ID"))
			})
		})

		t.Run("Format", func(t *ftt.Test) {
			assert.Loosely(t, TestExonerationName("a", "b", "ninja://chrome/test:foo_tests/BarTest.DoBaz", "some-id"),
				should.Equal(
					"rootInvocations/a/workUnits/b/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/exonerations/some-id"))
		})
	})
}
