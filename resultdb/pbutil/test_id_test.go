// Copyright 2025 The LUCI Authors.
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

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestParseAndValidateTestID(t *testing.T) {
	t.Parallel()
	ftt.Run("ParseAndValidateTestID", t, func(t *ftt.Test) {
		t.Run("Empty", func(t *ftt.Test) {
			_, err := ParseAndValidateTestID("")
			assert.Loosely(t, err, should.ErrLike("unspecified"))
		})
		t.Run("Too Long", func(t *ftt.Test) {
			_, err := ParseAndValidateTestID(strings.Repeat("a", 513))
			assert.Loosely(t, err, should.ErrLike("longer than 512 bytes"))
		})
		t.Run("Not valid UTF-8", func(t *ftt.Test) {
			_, err := ParseAndValidateTestID("\xbd")
			assert.Loosely(t, err, should.ErrLike("not a valid utf8 string"))
		})
		t.Run("Non-Printable", func(t *ftt.Test) {
			_, err := ParseAndValidateTestID("abc\u0000def")
			assert.Loosely(t, err, should.ErrLike("non-printable rune"))
		})
		t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
			_, err := ParseAndValidateTestID("e\u0301")
			assert.Loosely(t, err, should.ErrLike("not in unicode normalized form C"))
		})
		t.Run("Legacy", func(t *ftt.Test) {
			t.Run("Simple", func(t *ftt.Test) {
				id, err := ParseAndValidateTestID("valid_legacy_id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, id, should.Match(TestIdentifier{
					ModuleName:   "legacy",
					ModuleScheme: "legacy",
					CaseName:     "valid_legacy_id",
				}))
			})
			t.Run("Realistic", func(t *ftt.Test) {
				id, err := ParseAndValidateTestID("ninja://:blink_web_tests/http/tests/inspector-protocol/network/response-interception-request-completes-network-closes.js")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, id, should.Match(TestIdentifier{
					ModuleName:   "legacy",
					ModuleScheme: "legacy",
					CaseName:     "ninja://:blink_web_tests/http/tests/inspector-protocol/network/response-interception-request-completes-network-closes.js",
				}))
			})
		})
		t.Run("Structured", func(t *ftt.Test) {
			t.Run("Valid", func(t *ftt.Test) {
				id, err := ParseAndValidateTestID(":module!scheme:coarse:fine#case")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, id, should.Match(TestIdentifier{
					ModuleName:   "module",
					ModuleScheme: "scheme",
					CoarseName:   "coarse",
					FineName:     "fine",
					CaseName:     "case",
				}))
			})
			t.Run("Valid with Escapes", func(t *ftt.Test) {
				id, err := ParseAndValidateTestID(`:module\:!sch3m3:coarse\#:fine\:\!#case\\`)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, id, should.Match(TestIdentifier{
					ModuleName:   "module:",
					ModuleScheme: "sch3m3",
					CoarseName:   "coarse#",
					FineName:     "fine:!",
					CaseName:     `case\`,
				}))
			})
			t.Run("Invalid structure", func(t *ftt.Test) {
				t.Run("Missing scheme", func(t *ftt.Test) {
					_, err := ParseAndValidateTestID(":module")
					assert.Loosely(t, err, should.ErrLike("unexpected end of string at byte 7, expected delimeter '!' (test ID pattern is :module!scheme:coarse:fine#case)"))
				})
				t.Run("Missing coarse name", func(t *ftt.Test) {
					_, err := ParseAndValidateTestID(":module!scheme")
					assert.Loosely(t, err, should.ErrLike("unexpected end of string at byte 14, expected delimeter ':' (test ID pattern is :module!scheme:coarse:fine#case)"))
				})
				t.Run("Missing fine name", func(t *ftt.Test) {
					_, err := ParseAndValidateTestID(":module!scheme:coarse")
					assert.Loosely(t, err, should.ErrLike("unexpected end of string at byte 21, expected delimeter ':' (test ID pattern is :module!scheme:coarse:fine#case)"))
				})
				t.Run("Missing case name", func(t *ftt.Test) {
					_, err := ParseAndValidateTestID(":module!scheme:coarse:fine")
					assert.Loosely(t, err, should.ErrLike("unexpected end of string at byte 26, expected delimeter '#' (test ID pattern is :module!scheme:coarse:fine#case)"))
				})
			})
			t.Run("Invalid escape", func(t *ftt.Test) {
				_, err := ParseAndValidateTestID(`:module!scheme:coarse:fine#case\a`)
				assert.Loosely(t, err, should.ErrLike("got unexpected character 'a' at byte 32, while processing escape sequence (\\); only the characters :!#\\ may be escaped"))
			})
			t.Run("Unfinished escape", func(t *ftt.Test) {
				_, err := ParseAndValidateTestID(`:module!scheme:coarse:fine#case\`)
				assert.Loosely(t, err, should.ErrLike("unfinished escape sequence at byte 32, got end of string; expected one of :!#\\"))
			})
			t.Run("Invalid delimeter when expected different delimeter", func(t *ftt.Test) {
				_, err := ParseAndValidateTestID(`:module:scheme:coarse:fine#case:`)
				assert.Loosely(t, err, should.ErrLike("got delimeter character ':' at byte 7; expected normal character, escape sequence or delimeter '!' (test ID pattern is :module!scheme:coarse:fine#case)"))
			})
			t.Run("Invalid delimeter when expected end of string", func(t *ftt.Test) {
				_, err := ParseAndValidateTestID(`:module!scheme:coarse:fine#case:`)
				assert.Loosely(t, err, should.ErrLike("got delimeter character ':' at byte 31; expected normal character, escape sequence or end of string (test ID pattern is :module!scheme:coarse:fine#case)"))
			})
		})
	})
}

func TestValidateTestIdentifier(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateTestIdentifier", t, func(t *ftt.Test) {
		t.Run("Empty", func(t *ftt.Test) {
			err := validateTestIdentifier(TestIdentifier{})
			assert.Loosely(t, err, should.ErrLike("module_name: unspecified"))
		})
		id := TestIdentifier{
			ModuleName:   "module",
			ModuleScheme: "scheme",
			CoarseName:   "coarse",
			FineName:     "fine",
			CaseName:     "case",
		}
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
		})
		t.Run("Module Name", func(t *ftt.Test) {
			t.Run("Too Long", func(t *ftt.Test) {
				id.ModuleName = strings.Repeat("a", 301)
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("module_name: longer than 300 bytes"))
			})
			t.Run("Non-Printable", func(t *ftt.Test) {
				id.ModuleName = "abc\u0000def"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("module_name: non-printable rune"))
			})
			t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
				id.ModuleName = "e\u0301"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("module_name: not in unicode normalized form C"))
			})
			t.Run("Not valid UTF-8", func(t *ftt.Test) {
				id.ModuleName = "\xbd"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("module_name: not a valid utf8 string"))
			})
		})
		t.Run("Module Scheme", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				id.ModuleScheme = ""
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("module_scheme: unspecified"))
			})
			t.Run("Too Long", func(t *ftt.Test) {
				id.ModuleScheme = strings.Repeat("a", 21)
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("module_scheme: longer than 20 bytes"))
			})
			t.Run("Invalid", func(t *ftt.Test) {
				id.ModuleScheme = "A"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike(`module_scheme: does not match "^[a-z][a-z0-9]*$"`))
			})
		})
		t.Run("Coarse Name", func(t *ftt.Test) {
			t.Run("Empty with fine name", func(t *ftt.Test) {
				id.CoarseName = ""
				id.FineName = "fine"
				assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
			})
			t.Run("Empty without fine name", func(t *ftt.Test) {
				id.CoarseName = ""
				id.FineName = ""
				assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
			})
			t.Run("Too Long", func(t *ftt.Test) {
				id.CoarseName = strings.Repeat("a", 301)
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("coarse_name: longer than 300 bytes"))
			})
			t.Run("Non-Printable", func(t *ftt.Test) {
				id.CoarseName = "abc\u0000def"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("coarse_name: non-printable rune"))
			})
			t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
				id.CoarseName = "e\u0301"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("coarse_name: not in unicode normalized form C"))
			})
			t.Run("Not valid UTF-8", func(t *ftt.Test) {
				id.CoarseName = "\xbd"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("coarse_name: not a valid utf8 string"))
			})
			t.Run("Reserved leading character", func(t *ftt.Test) {
				t.Run("Reserved character prior to ,", func(t *ftt.Test) {
					id.CoarseName = "+"
					assert.Loosely(t, validateTestIdentifier(id), should.ErrLike(`coarse_name: character '+' may not be used as a leading character of a coarse or fine name`))
				})
				t.Run("Last reserved character (,)", func(t *ftt.Test) {
					id.CoarseName = ","
					assert.Loosely(t, validateTestIdentifier(id), should.ErrLike(`coarse_name: character ',' may not be used as a leading character of a coarse or fine name`))
				})
				t.Run("First non-reserved character (-)", func(t *ftt.Test) {
					id.CoarseName = "-"
					assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
				})
			})
		})
		t.Run("Fine Name", func(t *ftt.Test) {
			t.Run("Empty without coarse name", func(t *ftt.Test) {
				id.CoarseName = ""
				id.FineName = ""
				assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
			})
			t.Run("Empty with coarse name", func(t *ftt.Test) {
				id.CoarseName = "coarse"
				id.FineName = ""
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("fine_name: unspecified when coarse_name is specified"))
			})

			t.Run("Too Long", func(t *ftt.Test) {
				id.FineName = strings.Repeat("a", 301)
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("fine_name: longer than 300 bytes"))
			})
			t.Run("Non-Printable", func(t *ftt.Test) {
				id.FineName = "abc\u0000def"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("fine_name: non-printable rune"))
			})
			t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
				id.FineName = "e\u0301"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("fine_name: not in unicode normalized form C"))
			})
			t.Run("Not valid UTF-8", func(t *ftt.Test) {
				id.FineName = "\xbd"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("fine_name: not a valid utf8 string"))
			})
			t.Run("Reserved leading characters", func(t *ftt.Test) {
				t.Run("Reserved character prior to ,", func(t *ftt.Test) {
					id.FineName = "+"
					assert.Loosely(t, validateTestIdentifier(id), should.ErrLike(`fine_name: character '+' may not be used as a leading character of a coarse or fine name`))
				})
				t.Run("Last reserved character (,)", func(t *ftt.Test) {
					id.FineName = ","
					assert.Loosely(t, validateTestIdentifier(id), should.ErrLike(`fine_name: character ',' may not be used as a leading character of a coarse or fine name`))
				})
				t.Run("First non-reserved character (-)", func(t *ftt.Test) {
					id.FineName = "-"
					assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
				})
			})
		})
		t.Run("Case Name", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				id.CaseName = ""
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("case_name: unspecified"))
			})
			t.Run("Too Long", func(t *ftt.Test) {
				id.CaseName = strings.Repeat("a", 513)
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("case_name: longer than 512 bytes"))
			})
			t.Run("Non-Printable", func(t *ftt.Test) {
				id.CaseName = "abc\u0000def"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("case_name: non-printable rune"))
			})
			t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
				id.CaseName = "e\u0301"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("case_name: not in unicode normalized form C"))
			})
			t.Run("Not valid UTF-8", func(t *ftt.Test) {
				id.CaseName = "\xbd"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("case_name: not a valid utf8 string"))
			})
			t.Run("Reserved names", func(t *ftt.Test) {
				t.Run("Reserved character prior to ,", func(t *ftt.Test) {
					id.CaseName = "!"
					assert.Loosely(t, validateTestIdentifier(id), should.ErrLike(`case_name: character '!' may not be used as a leading character of a case name`))
				})
				t.Run("Last reserved character (,)", func(t *ftt.Test) {
					id.CaseName = ","
					assert.Loosely(t, validateTestIdentifier(id), should.ErrLike(`case_name: character ',' may not be used as a leading character of a case name`))
				})
				t.Run("*", func(t *ftt.Test) {
					id.CaseName = "*"
					assert.Loosely(t, validateTestIdentifier(id), should.ErrLike(`case_name: character * may not be used as a leading character of a case name, unless the case name is '*fixture'`))
				})
				t.Run("*fixture", func(t *ftt.Test) {
					id.CaseName = "*fixture"
					assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
				})
			})
		})
		t.Run("Legacy", func(t *ftt.Test) {
			id.ModuleName = "legacy"
			id.ModuleScheme = "legacy"
			id.CoarseName = ""
			id.FineName = ""
			t.Run("Valid", func(t *ftt.Test) {
				assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
			})
			t.Run("Invalid scheme", func(t *ftt.Test) {
				id.ModuleScheme = "notlegacy"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("module_scheme: must be set to 'legacy' for tests in the 'legacy' module"))
			})
			t.Run("Invalid coarse name", func(t *ftt.Test) {
				id.CoarseName = "coarse"
				id.FineName = "fine" // Must be specified to get past "unspecified when coarse_name is specified" error.
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("coarse_name: must be empty for tests in the 'legacy' module"))
			})
			t.Run("Invalid fine name", func(t *ftt.Test) {
				id.FineName = "fine"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("fine_name: must be empty for tests in the 'legacy' module"))
			})
			t.Run("Invalid case name", func(t *ftt.Test) {
				id.CaseName = ":case"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("case_name: must not start with ':' for tests in the 'legacy' module"))
			})
			t.Run("Invalid case name reserved", func(t *ftt.Test) {
				id.CaseName = "*case"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("case_name: must not start with '*' for tests in the 'legacy' module"))
			})
		})
		t.Run("Structured", func(t *ftt.Test) {
			t.Run("Invalid scheme", func(t *ftt.Test) {
				id.ModuleName = "module"
				id.ModuleScheme = "legacy"
				assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("module_scheme: must not be set to 'legacy' except for tests in the 'legacy' module"))
			})
		})
		t.Run("Limit on encoded length", func(t *ftt.Test) {
			t.Run("Without escaping", func(t *ftt.Test) {
				id.ModuleName = strings.Repeat("a", 100)
				id.ModuleScheme = "scheme1"
				id.CoarseName = strings.Repeat("b", 100)
				id.FineName = strings.Repeat("c", 100)
				id.CaseName = strings.Repeat("d", 200)
				t.Run("Not too long", func(t *ftt.Test) {
					assert.Loosely(t, sizeEscapedTestID(id), should.Equal(512))
					assert.Loosely(t, len(EncodeTestID(id)), should.Equal(512))
					assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
				})
				t.Run("Too long", func(t *ftt.Test) {
					id.CaseName += "d"
					assert.Loosely(t, sizeEscapedTestID(id), should.Equal(513))
					assert.Loosely(t, len(EncodeTestID(id)), should.Equal(513))
					assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("test ID exceeds 512 bytes in encoded form"))
				})
			})
			t.Run("With escaping", func(t *ftt.Test) {
				id.ModuleName = strings.Repeat(":", 50)
				id.ModuleScheme = "scheme1"
				id.CoarseName = "aa" + strings.Repeat("!", 49) // ! may not be used as a leading character in a coarse name.
				id.FineName = "aa" + strings.Repeat("#", 49)   // # may not be used as a leading character in a fine name.
				id.CaseName = strings.Repeat("\\", 100)
				t.Run("Not too long", func(t *ftt.Test) {
					assert.Loosely(t, sizeEscapedTestID(id), should.Equal(512))
					assert.Loosely(t, len(EncodeTestID(id)), should.Equal(512))
					assert.Loosely(t, validateTestIdentifier(id), should.BeNil)
				})
				t.Run("Too long", func(t *ftt.Test) {
					id.CaseName += "d"
					assert.Loosely(t, sizeEscapedTestID(id), should.Equal(513))
					assert.Loosely(t, len(EncodeTestID(id)), should.Equal(513))
					assert.Loosely(t, validateTestIdentifier(id), should.ErrLike("test ID exceeds 512 bytes in encoded form"))
				})
			})
		})
	})
}

func TestValidateTestVariantIdentifier(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateTestVariantIdentifier", t, func(t *ftt.Test) {
		t.Run("Nil", func(t *ftt.Test) {
			assert.Loosely(t, ValidateTestVariantIdentifier(nil), should.ErrLike("unspecified"))
		})
		id := &pb.TestVariantIdentifier{
			ModuleName:   "module",
			ModuleScheme: "scheme",
			CoarseName:   "coarse",
			FineName:     "fine",
			CaseName:     "case",
			ModuleVariant: &pb.Variant{
				Def: map[string]string{
					"k": "v",
				},
			},
		}

		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateTestVariantIdentifier(id), should.BeNil)
		})
		t.Run("Test ID Components", func(t *ftt.Test) {
			// Validation of Test ID components is delegated to ValidateTestIdentifier which
			// has its own test cases above. Simply test that each field is correctly passed through
			// to that method.
			t.Run("Module name", func(t *ftt.Test) {
				id.ModuleName = ""
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.ErrLike("module_name: unspecified"))
			})
			t.Run("Module scheme", func(t *ftt.Test) {
				id.ModuleScheme = ""
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.ErrLike("module_scheme: unspecified"))
			})
			t.Run("Coarse name", func(t *ftt.Test) {
				id.CoarseName = "\u0000"
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.ErrLike("coarse_name: non-printable rune '\\x00' at byte index 0"))
			})
			t.Run("Fine name", func(t *ftt.Test) {
				id.FineName = "\u0000"
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.ErrLike("fine_name: non-printable rune '\\x00' at byte index 0"))
			})
			t.Run("Case name", func(t *ftt.Test) {
				id.CaseName = ""
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.ErrLike("case_name: unspecified"))
			})
		})
		t.Run("Module Variant", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				id.ModuleVariant = nil
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.BeNil)
			})
			t.Run("Invalid", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"\x00": "v",
					},
				}
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.ErrLike("module_variant: \"\\x00\":\"v\": key: does not match pattern"))
			})
		})
		t.Run("Module Variant Hash", func(t *ftt.Test) {
			t.Run("Unset", func(t *ftt.Test) {
				// It is valid to leave the hash unset. It can be computed from
				// the variant if necessary.
				id.ModuleVariantHash = ""
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.BeNil)
			})
			t.Run("Set and matches", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"k": "v",
					},
				}
				id.ModuleVariantHash = "b1618cc2bf370a7c"
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.BeNil)
			})
			t.Run("Set and mismatches", func(t *ftt.Test) {
				id.ModuleVariantHash = "0a0a0a0a0a0a0a0a"
				assert.Loosely(t, ValidateTestVariantIdentifier(id), should.ErrLike("module_variant_hash: expected b1618cc2bf370a7c to match specified variant"))
			})
		})
	})
}

func TestEncodeTestID(t *testing.T) {
	t.Parallel()
	ftt.Run("EncodeTestID", t, func(t *ftt.Test) {
		t.Run("Legacy", func(t *ftt.Test) {
			id := TestIdentifier{
				ModuleName:   "legacy",
				ModuleScheme: "legacy",
				CaseName:     "case",
			}
			encodedID := EncodeTestID(id)
			assert.Loosely(t, encodedID, should.Equal("case"))

			parsedID, err := ParseAndValidateTestID(encodedID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsedID, should.Match(id))
		})
		t.Run("Legacy with special characters", func(t *ftt.Test) {
			id := TestIdentifier{
				ModuleName:   "legacy",
				ModuleScheme: "legacy",
				CaseName:     "mytest:amazing!blah.html#someReference\u0123",
			}
			encodedID := EncodeTestID(id)
			assert.Loosely(t, encodedID, should.Equal("mytest:amazing!blah.html#someReference\u0123"))

			parsedID, err := ParseAndValidateTestID(encodedID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsedID, should.Match(id))
		})
		t.Run("Structured", func(t *ftt.Test) {
			id := TestIdentifier{
				ModuleName:   "module",
				ModuleScheme: "scheme",
				CoarseName:   "coarse",
				FineName:     "fine",
				CaseName:     "case",
			}
			encodedID := EncodeTestID(id)
			assert.Loosely(t, encodedID, should.Equal(":module!scheme:coarse:fine#case"))

			parsedID, err := ParseAndValidateTestID(encodedID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsedID, should.Match(id))
		})
		t.Run("Structured with special characters", func(t *ftt.Test) {
			id := TestIdentifier{
				ModuleName:   "module",
				ModuleScheme: "scheme",
				CoarseName:   `mytest:amazing!`,
				FineName:     `path\to\blah.html#subreference`,
				CaseName:     "?test=\u0123",
			}
			encodedID := EncodeTestID(id)
			assert.Loosely(t, encodedID, should.Equal(`:module!scheme:mytest\:amazing\!:path\\to\\blah.html\#subreference#?test=ģ`))

			parsedID, err := ParseAndValidateTestID(encodedID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsedID, should.Match(id))
		})
	})
}

// These parsing, validation and encoding methods are called many billions of times per day.
// As such, every microsecond is equivalent to a few hours of CPU time per day and up to a
// few milliseconds per batch RPC.
//
// We benchmark them to ensure they are reasonably efficient.

// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
// BenchmarkValidateTestVariantIdentifier-96    	  401269	      2847 ns/op	     297 B/op	      14 allocs/op
func BenchmarkValidateTestVariantIdentifier(b *testing.B) {
	id := &pb.TestVariantIdentifier{
		ModuleName:   "android_webview/test:android_webview_junit_tests",
		ModuleScheme: "junit",
		CoarseName:   "org.chromium.android_webview.robolectric",
		FineName:     "AwDisplayModeControllerTest",
		CaseName:     "testFullscreen[28]",
		ModuleVariant: &pb.Variant{
			Def: map[string]string{
				"builder": "android-10-x86-fyi-rel",
			},
		},
	}
	id.ModuleVariantHash = VariantHash(&pb.Variant{
		Def: map[string]string{
			"builder": "android-10-x86-fyi-rel",
		},
	})
	for i := 0; i < b.N; i++ {
		err := ValidateTestVariantIdentifier(id)
		if err != nil {
			panic(err)
		}
	}
}

// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
// BenchmarkParseAndValidateTestIDLegacy-96    	  616791	      1965 ns/op	      48 B/op	       6 allocs/op
func BenchmarkParseAndValidateTestIDLegacy(b *testing.B) {
	// Tests a legacy test identifier.
	testID := "ninja://android_webview/test:android_webview_junit_tests/org.chromium.android_webview.robolectric.AwDisplayModeControllerTest#testFullscreen[28]"
	for i := 0; i < b.N; i++ {
		_, err := ParseAndValidateTestID(testID)
		if err != nil {
			panic(err)
		}
	}
}

// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
// BenchmarkParseAndValidateTestID-96    	  327799	      3558 ns/op	     233 B/op	      12 allocs/op
func BenchmarkParseAndValidateTestID(b *testing.B) {
	// Tests a structured test identifier in flat-form.
	testID := ":android_webview/test\\:android_webview_junit_tests!junit:org.chromium.android_webview.robolectric:AwDisplayModeControllerTest#testFullscreen[28]"
	for i := 0; i < b.N; i++ {
		_, err := ParseAndValidateTestID(testID)
		if err != nil {
			panic(err)
		}
	}
}

// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
// BenchmarkEncode-96    	 1987162	       600.6 ns/op	     144 B/op	       1 allocs/op
func BenchmarkEncode(b *testing.B) {
	id := TestIdentifier{
		ModuleName:   "android_webview/test:android_webview_junit_tests",
		ModuleScheme: "junit",
		CoarseName:   "org.chromium.android_webview.robolectric",
		FineName:     "AwDisplayModeControllerTest",
		CaseName:     "testFullscreen[28]",
	}
	for i := 0; i < b.N; i++ {
		EncodeTestID(id)
	}
}
