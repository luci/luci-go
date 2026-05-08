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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
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

func TestValidateTestExoneration(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateTestExoneration`, t, func(t *ftt.Test) {
		var invoked bool
		validateToScheme := func(id BaseTestIdentifier) error {
			invoked = true
			if id.ModuleScheme == "junit" && id.CoarseName == "" {
				return fmt.Errorf("coarse_name: required, please set a Package (scheme \"junit\")")
			}
			return nil
		}
		validateTE := func(ex *pb.TestExoneration, strict bool) error {
			return ValidateTestExoneration(ex, validateToScheme, DefaultTestIDLimitCallback, strict)
		}

		t.Run(`Unspecified`, func(t *ftt.Test) {
			err := validateTE(nil, true)
			assert.Loosely(t, err, should.ErrLike(`unspecified`))
		})

		t.Run(`nil validateToScheme`, func(t *ftt.Test) {
			assert.Loosely(t, func() { ValidateTestExoneration(&pb.TestExoneration{}, nil, DefaultTestIDLimitCallback, true) }, should.PanicLikeString("validateToScheme is required"))
		})

		t.Run(`Both flat and structured Test ID unspecified`, func(t *ftt.Test) {
			err := validateTE(&pb.TestExoneration{}, true)
			assert.Loosely(t, err, should.ErrLike("test_id_structured: unspecified"))
		})

		ex := &pb.TestExoneration{
			TestIdStructured: &pb.TestIdentifier{
				ModuleName:   "//infra/junit_tests",
				ModuleScheme: "junit",
				ModuleVariant: Variant(
					"key", "value",
				),
				CoarseName: "org.chromium.go.luci",
				FineName:   "ValidationTests",
				CaseName:   "FooBar",
			},
			ExplanationHtml: "Unexpected pass.",
			Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
		}
		t.Run(`Valid`, func(t *ftt.Test) {
			invoked = false
			err := validateTE(ex, true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invoked, should.BeTrue)
		})
		t.Run("Structured test identifier", func(t *ftt.Test) {
			t.Run("Structure", func(t *ftt.Test) {
				t.Run(`Invalid case name`, func(t *ftt.Test) {
					ex.TestIdStructured.CaseName = "case name \x00"
					assert.Loosely(t, validateTE(ex, true), should.ErrLike("test_id_structured: case_name: non-printable rune '\\x00' at byte index 10"))
				})
				t.Run(`Invalid module variant`, func(t *ftt.Test) {
					ex.TestIdStructured.ModuleVariant = Variant("key\x00", "value")
					assert.Loosely(t, validateTE(ex, true), should.ErrLike("test_id_structured: module_variant: \"key\\x00\":\"value\": key: does not match pattern"))
				})
			})
			t.Run("Scheme", func(t *ftt.Test) {
				t.Run(`Coarse name missing`, func(t *ftt.Test) {
					ex.TestIdStructured = nil
					ex.TestId = ":myModule!junit::Class#Method"
					ex.Variant = Variant("a", "1", "b", "2")
					assert.Loosely(t, validateTE(ex, true), should.ErrLike("test_id: coarse_name: required, please set a Package (scheme \"junit\")"))
				})
			})
		})
		t.Run("Legacy", func(t *ftt.Test) {
			t.Run("Legacy fields are ignored unless structured test identifier cleared", func(t *ftt.Test) {
				ex.TestId = "something\x00"
				ex.Variant = Variant("", "")
				err := validateTE(ex, true)
				assert.Loosely(t, err, should.BeNil)
			})

			ex.TestIdStructured = nil
			ex.TestId = "something"
			ex.Variant = Variant("a", "1", "b", "2")

			t.Run(`Valid`, func(t *ftt.Test) {
				invoked = false
				err := validateTE(ex, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, invoked, should.BeTrue)
			})
			t.Run("Test ID", func(t *ftt.Test) {
				t.Run(`Non-printable runes`, func(t *ftt.Test) {
					ex.TestId = "\x01"
					err := validateTE(ex, true)
					assert.Loosely(t, err, should.ErrLike("test_id: non-printable rune"))
				})
			})
			t.Run("Variant/VariantHash", func(t *ftt.Test) {
				t.Run(`Mismatching variant hashes`, func(t *ftt.Test) {
					ex.VariantHash = "doesn't match"
					err := validateTE(ex, true)
					assert.Loosely(t, err, should.ErrLike(`computed and supplied variant hash don't match`))
				})
				t.Run(`Matching variant hashes`, func(t *ftt.Test) {
					ex.Variant = Variant("a", "b")
					ex.VariantHash = "c467ccce5a16dc72"
					err := validateTE(ex, true)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`Variant hash only`, func(t *ftt.Test) {
					ex.Variant = nil
					ex.VariantHash = "c467ccce5a16dc72"

					t.Run(`Allowed in unstrict mode`, func(t *ftt.Test) {
						err := validateTE(ex, false)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run(`Disallowed in strict mode`, func(t *ftt.Test) {
						err := validateTE(ex, true)
						assert.Loosely(t, err, should.ErrLike(`variant: unspecified`))
					})
				})
				t.Run(`Invalid variant`, func(t *ftt.Test) {
					ex.Variant = Variant("", "")
					err := validateTE(ex, true)
					assert.Loosely(t, err, should.ErrLike(`variant: "":"": key: unspecified`))
				})
			})
		})
		t.Run(`Reason is not specified`, func(t *ftt.Test) {
			ex.Reason = pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED
			err := validateTE(ex, true)
			assert.Loosely(t, err, should.ErrLike(`reason: unspecified`))
		})
		t.Run(`Explanation HTML not specified`, func(t *ftt.Test) {
			ex.ExplanationHtml = ""
			err := validateTE(ex, true)
			assert.Loosely(t, err, should.ErrLike(`explanation_html: unspecified`))
		})
	})
}
