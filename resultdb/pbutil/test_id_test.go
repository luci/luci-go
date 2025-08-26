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
				assert.Loosely(t, id, should.Match(BaseTestIdentifier{
					ModuleName:   "legacy",
					ModuleScheme: "legacy",
					CaseName:     "valid_legacy_id",
				}))
			})
			t.Run("Realistic", func(t *ftt.Test) {
				id, err := ParseAndValidateTestID("ninja://:blink_web_tests/http/tests/inspector-protocol/network/response-interception-request-completes-network-closes.js")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, id, should.Match(BaseTestIdentifier{
					ModuleName:   "legacy",
					ModuleScheme: "legacy",
					CaseName:     "ninja://:blink_web_tests/http/tests/inspector-protocol/network/response-interception-request-completes-network-closes.js",
				}))
			})
			t.Run("Non-roundtrippable encodings illegal", func(t *ftt.Test) {
				_, err := ParseAndValidateTestID(":legacy!legacy::#method")
				assert.Loosely(t, err, should.ErrLike(`module "legacy" may not be used within a structured test ID encoding`))
			})
		})
		t.Run("Structured", func(t *ftt.Test) {
			t.Run("Valid", func(t *ftt.Test) {
				id, err := ParseAndValidateTestID(":module!scheme:coarse:fine#case")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, id, should.Match(BaseTestIdentifier{
					ModuleName:   "module",
					ModuleScheme: "scheme",
					CoarseName:   "coarse",
					FineName:     "fine",
					CaseName:     "case",
				}))
			})
			t.Run("Valid with multi-part case name", func(t *ftt.Test) {
				id, err := ParseAndValidateTestID(":module!scheme:coarse:fine#case:part2:part3")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, id, should.Match(BaseTestIdentifier{
					ModuleName:   "module",
					ModuleScheme: "scheme",
					CoarseName:   "coarse",
					FineName:     "fine",
					CaseName:     "case:part2:part3",
				}))
			})
			t.Run("Valid with Escapes", func(t *ftt.Test) {
				id, err := ParseAndValidateTestID(`:module\:!sch3m3:coarse\#:fine\:\!#case\:\\\#\!:subCase`)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, id, should.Match(BaseTestIdentifier{
					ModuleName:   "module:",
					ModuleScheme: "sch3m3",
					CoarseName:   "coarse#",
					FineName:     "fine:!",
					CaseName:     `case\:\\#!:subCase`,
				}))
			})
			t.Run("Invalid structure", func(t *ftt.Test) {
				t.Run("Missing scheme", func(t *ftt.Test) {
					_, err := ParseAndValidateTestID(":module")
					assert.Loosely(t, err, should.ErrLike("unexpected end of string at byte 7, expected delimiter '!' (test ID pattern is :module!scheme:coarse:fine#case)"))
				})
				t.Run("Missing coarse name", func(t *ftt.Test) {
					_, err := ParseAndValidateTestID(":module!scheme")
					assert.Loosely(t, err, should.ErrLike("unexpected end of string at byte 14, expected delimiter ':' (test ID pattern is :module!scheme:coarse:fine#case)"))
				})
				t.Run("Missing fine name", func(t *ftt.Test) {
					_, err := ParseAndValidateTestID(":module!scheme:coarse")
					assert.Loosely(t, err, should.ErrLike("unexpected end of string at byte 21, expected delimiter ':' (test ID pattern is :module!scheme:coarse:fine#case)"))
				})
				t.Run("Missing case name", func(t *ftt.Test) {
					_, err := ParseAndValidateTestID(":module!scheme:coarse:fine")
					assert.Loosely(t, err, should.ErrLike("unexpected end of string at byte 26, expected delimiter '#' (test ID pattern is :module!scheme:coarse:fine#case)"))
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
			t.Run("Invalid delimiter when expected different delimiter", func(t *ftt.Test) {
				_, err := ParseAndValidateTestID(`:module:scheme:coarse:fine#case`)
				assert.Loosely(t, err, should.ErrLike("got delimiter character ':' at byte 7; expected normal character, escape sequence or delimiter '!' (test ID pattern is :module!scheme:coarse:fine#case)"))
			})
			t.Run("Invalid delimiter when expected end of string", func(t *ftt.Test) {
				_, err := ParseAndValidateTestID(`:module!scheme:coarse:fine#case#`)
				assert.Loosely(t, err, should.ErrLike("got delimiter character '#' at byte 31; expected normal character, escape sequence or end of string (test ID pattern is :module!scheme:coarse:fine#case)"))
			})
		})
	})
}

func TestValidateModuleName(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateModuleName", t, func(t *ftt.Test) {
		moduleName := "module"
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateModuleName(moduleName), should.BeNil)
		})
		t.Run("Unicode is allowed", func(t *ftt.Test) {
			moduleName = "µs-timing"
			assert.Loosely(t, ValidateModuleName(moduleName), should.BeNil)
		})
		t.Run("Too Long", func(t *ftt.Test) {
			moduleName = strings.Repeat("a", 301)
			assert.Loosely(t, ValidateModuleName(moduleName), should.ErrLike("longer than 300 bytes"))
		})
		t.Run("Non-Printable", func(t *ftt.Test) {
			moduleName = "abc\u0000def"
			assert.Loosely(t, ValidateModuleName(moduleName), should.ErrLike("non-printable rune"))
		})
		t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
			moduleName = "e\u0301"
			assert.Loosely(t, ValidateModuleName(moduleName), should.ErrLike("not in unicode normalized form C"))
		})
		t.Run("Not valid UTF-8", func(t *ftt.Test) {
			moduleName = "\xbd"
			assert.Loosely(t, ValidateModuleName(moduleName), should.ErrLike("not a valid utf8 string"))
		})
		t.Run("Error rune", func(t *ftt.Test) {
			moduleName = "aa\ufffd"
			assert.Loosely(t, ValidateModuleName(moduleName), should.ErrLike("unicode replacement character (U+FFFD) at byte index 2"))
		})
	})
}
func TestValidateModuleScheme(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateModuleScheme", t, func(t *ftt.Test) {
		scheme := "scheme"
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateModuleScheme(scheme, false), should.BeNil)
		})
		t.Run("Too Long", func(t *ftt.Test) {
			scheme = strings.Repeat("a", 21)
			assert.Loosely(t, ValidateModuleScheme(scheme, false), should.ErrLike("longer than 20 bytes"))
		})
		t.Run("Invalid", func(t *ftt.Test) {
			scheme = "A"
			assert.Loosely(t, ValidateModuleScheme(scheme, false), should.ErrLike(`does not match "^[a-z][a-z0-9]*$"`))
		})
		t.Run("Legacy is reserved", func(t *ftt.Test) {
			scheme = "legacy"
			assert.Loosely(t, ValidateModuleScheme(scheme, false), should.ErrLike(`must not be set to "legacy" except in the "legacy" module`))
		})
		t.Run("Legacy is allowed for legacy module", func(t *ftt.Test) {
			scheme = "legacy"
			assert.Loosely(t, ValidateModuleScheme(scheme, true), should.BeNil)
		})
		t.Run("Non-legacy is not allowed for legacy module", func(t *ftt.Test) {
			scheme = "scheme"
			assert.Loosely(t, ValidateModuleScheme(scheme, true), should.ErrLike(`must be set to "legacy" in the "legacy" module`))
		})
	})
}

func TestValidateBaseTestIdentifier(t *testing.T) {
	t.Parallel()
	ftt.Run("validateBaseTestIdentifier", t, func(t *ftt.Test) {
		t.Run("Empty", func(t *ftt.Test) {
			err := ValidateBaseTestIdentifier(BaseTestIdentifier{})
			assert.Loosely(t, err, should.ErrLike("module_name: unspecified"))
		})
		id := BaseTestIdentifier{
			ModuleName:   "module",
			ModuleScheme: "scheme",
			CoarseName:   "coarse",
			FineName:     "fine",
			CaseName:     "case",
		}
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
		})
		t.Run("Module Name", func(t *ftt.Test) {
			// The implementation calls into ValidateModuleName. That already has its own tests, so
			// just verify it is called.
			t.Run("Unicode is allowed", func(t *ftt.Test) {
				id.ModuleName = "µs-timing"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})
			t.Run("Error rune", func(t *ftt.Test) {
				id.ModuleName = "aa\ufffd"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("module_name: unicode replacement character (U+FFFD) at byte index 2"))
			})
		})
		t.Run("Module Scheme", func(t *ftt.Test) {
			// The implementation calls into ValidateModuleScheme. That already has its own tests, so
			// just verify it is called.
			t.Run("Legacy is reserved", func(t *ftt.Test) {
				id.ModuleName = "module"
				id.ModuleScheme = "legacy"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`module_scheme: must not be set to "legacy" except in the "legacy" module`))
			})
		})
		t.Run("Coarse Name", func(t *ftt.Test) {
			t.Run("Unicode is allowed", func(t *ftt.Test) {
				id.CoarseName = "µs"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})
			t.Run("Empty with fine name", func(t *ftt.Test) {
				id.CoarseName = ""
				id.FineName = "fine"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})
			t.Run("Empty without fine name", func(t *ftt.Test) {
				id.CoarseName = ""
				id.FineName = ""
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})
			t.Run("Too Long", func(t *ftt.Test) {
				id.CoarseName = strings.Repeat("a", 301)
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("coarse_name: longer than 300 bytes"))
			})
			t.Run("Non-Printable", func(t *ftt.Test) {
				id.CoarseName = "abc\u0000def"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("coarse_name: non-printable rune"))
			})
			t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
				id.CoarseName = "e\u0301"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("coarse_name: not in unicode normalized form C"))
			})
			t.Run("Not valid UTF-8", func(t *ftt.Test) {
				id.CoarseName = "\xbd"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("coarse_name: not a valid utf8 string"))
			})
			t.Run("Error rune", func(t *ftt.Test) {
				id.CoarseName = "aa\ufffd"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("coarse_name: unicode replacement character (U+FFFD) at byte index 2"))
			})
			t.Run("Reserved leading character", func(t *ftt.Test) {
				t.Run("Reserved character prior to ,", func(t *ftt.Test) {
					id.CoarseName = "+"
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`coarse_name: character '+' may not be used as a leading character of a coarse or fine name`))
				})
				t.Run("Last reserved character (,)", func(t *ftt.Test) {
					id.CoarseName = ","
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`coarse_name: character ',' may not be used as a leading character of a coarse or fine name`))
				})
				t.Run("First non-reserved character (-)", func(t *ftt.Test) {
					id.CoarseName = "-"
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
				})
			})
		})
		t.Run("Fine Name", func(t *ftt.Test) {
			t.Run("Unicode is allowed", func(t *ftt.Test) {
				id.FineName = "µs"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})
			t.Run("Empty without coarse name", func(t *ftt.Test) {
				id.CoarseName = ""
				id.FineName = ""
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})
			t.Run("Empty with coarse name", func(t *ftt.Test) {
				id.CoarseName = "coarse"
				id.FineName = ""
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("fine_name: unspecified when coarse_name is specified"))
			})

			t.Run("Too Long", func(t *ftt.Test) {
				id.FineName = strings.Repeat("a", 301)
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("fine_name: longer than 300 bytes"))
			})
			t.Run("Non-Printable", func(t *ftt.Test) {
				id.FineName = "abc\u0000def"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("fine_name: non-printable rune"))
			})
			t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
				id.FineName = "e\u0301"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("fine_name: not in unicode normalized form C"))
			})
			t.Run("Not valid UTF-8", func(t *ftt.Test) {
				id.FineName = "\xbd"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("fine_name: not a valid utf8 string"))
			})
			t.Run("Error rune", func(t *ftt.Test) {
				id.FineName = "aa\ufffd"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("fine_name: unicode replacement character (U+FFFD) at byte index 2"))
			})
			t.Run("Reserved leading characters", func(t *ftt.Test) {
				t.Run("Reserved character prior to ,", func(t *ftt.Test) {
					id.FineName = "+"
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`fine_name: character '+' may not be used as a leading character of a coarse or fine name`))
				})
				t.Run("Last reserved character (,)", func(t *ftt.Test) {
					id.FineName = ","
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`fine_name: character ',' may not be used as a leading character of a coarse or fine name`))
				})
				t.Run("First non-reserved character (-)", func(t *ftt.Test) {
					id.FineName = "-"
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
				})
			})
		})
		t.Run("Case Name", func(t *ftt.Test) {
			t.Run("Unicode is allowed", func(t *ftt.Test) {
				id.CaseName = "µs"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})
			t.Run("Empty", func(t *ftt.Test) {
				id.CaseName = ""
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: unspecified"))
			})
			t.Run("Too Long", func(t *ftt.Test) {
				id.CaseName = strings.Repeat("a", 513)
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: longer than 512 bytes"))
			})
			t.Run("Non-Printable", func(t *ftt.Test) {
				id.CaseName = "abc\u0000def"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: non-printable rune"))
			})
			t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
				id.CaseName = "e\u0301"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: not in unicode normalized form C"))
			})
			t.Run("Not valid UTF-8", func(t *ftt.Test) {
				id.CaseName = "\xbd"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: not a valid utf8 string"))
			})
			t.Run("Error rune", func(t *ftt.Test) {
				id.CaseName = "aa\ufffd"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: unicode replacement character (U+FFFD) at byte index 2"))
			})
			t.Run("Reserved names", func(t *ftt.Test) {
				t.Run("Reserved character prior to ,", func(t *ftt.Test) {
					id.CaseName = "!"
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`case_name: character '!' may not be used as a leading character of a case name`))
				})
				t.Run("Last reserved character (,)", func(t *ftt.Test) {
					id.CaseName = ","
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`case_name: character ',' may not be used as a leading character of a case name`))
				})
				t.Run("*", func(t *ftt.Test) {
					id.CaseName = "*"
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`case_name: character * may not be used as a leading character of a case name, unless the case name is '*fixture'`))
				})
				t.Run("*fixture", func(t *ftt.Test) {
					id.CaseName = "*fixture"
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
				})
			})
			t.Run("Multi-component", func(t *ftt.Test) {
				t.Run("Valid, multi-part", func(t *ftt.Test) {
					id.CaseName = EncodeCaseName("a", "b", "c")
					assert.Loosely(t, id.CaseName, should.Equal("a:b:c"))
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
				})
				t.Run("Valid, special characters", func(t *ftt.Test) {
					id.CaseName = `Colon\:Backslash\\:Colon\::MyTest`
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
				})
				t.Run("Empty part", func(t *ftt.Test) {
					t.Run("Leading", func(t *ftt.Test) {
						id.CaseName = ":b:c"
						assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: component 1 is empty, each component of the case name must be non-empty"))
					})
					t.Run("Middle", func(t *ftt.Test) {
						id.CaseName = "a::c"
						assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: component 2 is empty, each component of the case name must be non-empty"))
					})
					t.Run("Trailing", func(t *ftt.Test) {
						id.CaseName = "a:b:"
						assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: component 3 is empty, each component of the case name must be non-empty"))
					})
				})
				t.Run("Invalid escape sequence", func(t *ftt.Test) {
					id.CaseName = `\#`
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: got unexpected character '#' at byte 1, while processing escape sequence (\\); only the characters \\ and : may be escaped"))
				})
				t.Run("Unfinished escape sequence", func(t *ftt.Test) {
					id.CaseName = `a\`
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("case_name: unfinished escape sequence at byte 2, got end of string; expected one of \\ or :"))
				})
			})
		})
		t.Run("Legacy", func(t *ftt.Test) {
			id.ModuleName = "legacy"
			id.ModuleScheme = "legacy"
			id.CoarseName = ""
			id.FineName = ""
			t.Run("Valid", func(t *ftt.Test) {
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})
			t.Run("Unicode is allowed", func(t *ftt.Test) {
				id.CaseName = "TestVariousDeadlines/5µs"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})
			t.Run("Error rune is allowed", func(t *ftt.Test) {
				// Unicode replacement character.
				id.CaseName = "\ufffd"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
			})

			t.Run("Invalid scheme", func(t *ftt.Test) {
				id.ModuleScheme = "notlegacy"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`module_scheme: must be set to "legacy" in the "legacy" module`))
			})
			t.Run("Invalid coarse name", func(t *ftt.Test) {
				id.CoarseName = "coarse"
				id.FineName = "fine" // Must be specified to get past "unspecified when coarse_name is specified" error.
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`coarse_name: must be empty for tests in the "legacy" module`))
			})
			t.Run("Invalid fine name", func(t *ftt.Test) {
				id.FineName = "fine"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`fine_name: must be empty for tests in the "legacy" module`))
			})
			t.Run("Invalid case name", func(t *ftt.Test) {
				id.CaseName = ":case"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`case_name: must not start with ':' for tests in the "legacy" module`))
			})
			t.Run("Invalid case name reserved", func(t *ftt.Test) {
				id.CaseName = "*case"
				assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike(`case_name: must not start with '*' for tests in the "legacy" module`))
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
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
				})
				t.Run("Too long", func(t *ftt.Test) {
					id.CaseName += "d"
					assert.Loosely(t, sizeEscapedTestID(id), should.Equal(513))
					assert.Loosely(t, len(EncodeTestID(id)), should.Equal(513))
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("test ID exceeds 512 bytes in encoded form"))
				})
			})
			t.Run("With escaping", func(t *ftt.Test) {
				// 100 bytes (module name) + 7 (scheme) + 100 (coarse name) + 100 (fine name) + 200 (case name) + 5 for ":!::#"  = 512 bytes
				id.ModuleName = strings.Repeat(":", 50)        // Will be escaped, leading to 100 bytes used.
				id.ModuleScheme = "scheme1"                    // 7 bytes
				id.CoarseName = "aa" + strings.Repeat("!", 49) // ! may not be used as a leading character in a coarse name. 100 bytes when escaped.
				id.FineName = "aa" + strings.Repeat("#", 49)   // # may not be used as a leading character in a fine name. 100 bytes when escaped.
				id.CaseName = "aa" + strings.Repeat("#", 99)   // # may not be used as a leading character in a case name. 200 bytes when escaped.
				t.Run("Not too long", func(t *ftt.Test) {
					assert.Loosely(t, sizeEscapedTestID(id), should.Equal(512))
					assert.Loosely(t, len(EncodeTestID(id)), should.Equal(512))
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.BeNil)
				})
				t.Run("Too long", func(t *ftt.Test) {
					id.CaseName += "d"
					assert.Loosely(t, sizeEscapedTestID(id), should.Equal(513))
					assert.Loosely(t, len(EncodeTestID(id)), should.Equal(513))
					assert.Loosely(t, ValidateBaseTestIdentifier(id), should.ErrLike("test ID exceeds 512 bytes in encoded form"))
				})
			})
		})
	})
}

func TestValidateStructuredTestIdentifierForStorage(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateStructuredTestIdentifierForStorage", t, func(t *ftt.Test) {
		t.Run("Nil", func(t *ftt.Test) {
			assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(nil), should.ErrLike("unspecified"))
		})
		id := &pb.TestIdentifier{
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
			assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.BeNil)
		})
		t.Run("Test ID Components", func(t *ftt.Test) {
			// Validation of Test ID components is delegated to ValidateBaseTestIdentifier which
			// has its own test cases above. Simply test that each field is correctly passed through
			// to that method.
			t.Run("Module name", func(t *ftt.Test) {
				id.ModuleName = ""
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.ErrLike("module_name: unspecified"))
			})
			t.Run("Module scheme", func(t *ftt.Test) {
				id.ModuleScheme = ""
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.ErrLike("module_scheme: unspecified"))
			})
			t.Run("Coarse name", func(t *ftt.Test) {
				id.CoarseName = "\u0000"
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.ErrLike("coarse_name: non-printable rune '\\x00' at byte index 0"))
			})
			t.Run("Fine name", func(t *ftt.Test) {
				id.FineName = "\u0000"
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.ErrLike("fine_name: non-printable rune '\\x00' at byte index 0"))
			})
			t.Run("Case name", func(t *ftt.Test) {
				id.CaseName = ""
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.ErrLike("case_name: unspecified"))
			})
		})
		t.Run("Module Variant", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				id.ModuleVariant = nil
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.ErrLike("module_variant: unspecified"))
			})
			t.Run("Invalid", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"\x00": "v",
					},
				}
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.ErrLike("module_variant: \"\\x00\":\"v\": key: does not match pattern"))
			})
		})
		t.Run("Module Variant Hash", func(t *ftt.Test) {
			t.Run("Unset", func(t *ftt.Test) {
				// This is valid.
				id.ModuleVariantHash = ""
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.BeNil)
			})
			t.Run("Set and matches", func(t *ftt.Test) {
				// This is invalid, even though the hash matches.
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"k": "v",
					},
				}
				id.ModuleVariantHash = "b1618cc2bf370a7c"
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.BeNil)
			})
			t.Run("Set and mismatches", func(t *ftt.Test) {
				// This is invalid, even though the hash matches.
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"k": "v",
					},
				}
				id.ModuleVariantHash = "blah"
				assert.Loosely(t, ValidateStructuredTestIdentifierForStorage(id), should.ErrLike("module_variant_hash: expected b1618cc2bf370a7c (to match module_variant) or for value to be unset"))
			})
		})
	})
}

func TestValidateStructuredTestIdentifierForQuery(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateStructuredTestIdentifierForQuery", t, func(t *ftt.Test) {
		t.Run("Nil", func(t *ftt.Test) {
			assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(nil), should.ErrLike("unspecified"))
		})
		id := &pb.TestIdentifier{
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
			assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.BeNil)
		})
		t.Run("Test ID Components", func(t *ftt.Test) {
			// Validation of Test ID components is delegated to ValidateBaseTestIdentifier which
			// has its own test cases above. Simply test that each field is correctly passed through
			// to that method.
			t.Run("Module name", func(t *ftt.Test) {
				id.ModuleName = ""
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.ErrLike("module_name: unspecified"))
			})
			t.Run("Module scheme", func(t *ftt.Test) {
				id.ModuleScheme = ""
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.ErrLike("module_scheme: unspecified"))
			})
			t.Run("Coarse name", func(t *ftt.Test) {
				id.CoarseName = "\u0000"
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.ErrLike("coarse_name: non-printable rune '\\x00' at byte index 0"))
			})
			t.Run("Fine name", func(t *ftt.Test) {
				id.FineName = "\u0000"
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.ErrLike("fine_name: non-printable rune '\\x00' at byte index 0"))
			})
			t.Run("Case name", func(t *ftt.Test) {
				id.CaseName = ""
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.ErrLike("case_name: unspecified"))
			})
		})
		t.Run("Module variant and hash unset", func(t *ftt.Test) {
			id.ModuleVariant = nil
			id.ModuleVariantHash = ""
			assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.ErrLike("at least one of module_variant and module_variant_hash must be set"))
		})
		t.Run("Module Variant", func(t *ftt.Test) {
			t.Run("Empty but hash set", func(t *ftt.Test) {
				id.ModuleVariant = nil
				id.ModuleVariantHash = "b1618cc2bf370a7c"
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.BeNil)
			})
			t.Run("Invalid", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"\x00": "v",
					},
				}
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.ErrLike("module_variant: \"\\x00\":\"v\": key: does not match pattern"))
			})
		})
		t.Run("Module Variant Hash", func(t *ftt.Test) {
			t.Run("Unset", func(t *ftt.Test) {
				// It is valid to leave the hash unset. It can be computed from
				// the variant if necessary.
				id.ModuleVariantHash = ""
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.BeNil)
			})
			t.Run("Set and matches", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"k": "v",
					},
				}
				id.ModuleVariantHash = "b1618cc2bf370a7c"
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.BeNil)
			})
			t.Run("Set and mismatches", func(t *ftt.Test) {
				id.ModuleVariantHash = "0a0a0a0a0a0a0a0a"
				assert.Loosely(t, ValidateStructuredTestIdentifierForQuery(id), should.ErrLike("module_variant_hash: expected b1618cc2bf370a7c (to match module_variant) or for value to be unset"))
			})
		})
	})
}

func TestEncodeTestID(t *testing.T) {
	t.Parallel()
	ftt.Run("EncodeTestID", t, func(t *ftt.Test) {
		t.Run("Legacy", func(t *ftt.Test) {
			id := BaseTestIdentifier{
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
			id := BaseTestIdentifier{
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
			id := BaseTestIdentifier{
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
			id := BaseTestIdentifier{
				ModuleName:   "module",
				ModuleScheme: "scheme",
				CoarseName:   `mytest:amazing!`,
				FineName:     `path\to\blah.html#subreference`,
				CaseName:     EncodeCaseName("?test=\u0123!#", `withABackslash\andAColon:`),
			}
			encodedID := EncodeTestID(id)
			assert.Loosely(t, encodedID, should.Equal(`:module!scheme:mytest\:amazing\!:path\\to\\blah.html\#subreference#?test=ģ\!\#:withABackslash\\andAColon\:`))

			parsedID, err := ParseAndValidateTestID(encodedID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsedID, should.Match(id))
		})
	})
}

func TestEncodeCaseName(t *testing.T) {
	t.Parallel()
	ftt.Run("EncodeCaseName", t, func(t *ftt.Test) {
		t.Run("Single", func(t *ftt.Test) {
			assert.Loosely(t, EncodeCaseName("a"), should.Equal("a"))
		})
		t.Run("Multiple", func(t *ftt.Test) {
			assert.Loosely(t, EncodeCaseName("a", "b", "c"), should.Equal("a:b:c"))
		})
		t.Run("Escapes", func(t *ftt.Test) {
			assert.Loosely(t, EncodeCaseName(`a\:`, `b#!b`, `c:`), should.Equal(`a\\\::b#!b:c\:`))
		})
	})
}

func TestParseTestCaseName(t *testing.T) {
	t.Parallel()
	ftt.Run("ParseTestCaseName", t, func(t *ftt.Test) {
		t.Run("Single", func(t *ftt.Test) {
			parts, err := parseTestCaseName("a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parts, should.Match([]string{"a"}))
		})
		t.Run("Multiple", func(t *ftt.Test) {
			parts, err := parseTestCaseName("a:b:c")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parts, should.Match([]string{"a", "b", "c"}))
		})
		t.Run("Multiple with escapes", func(t *ftt.Test) {
			parts, err := parseTestCaseName(`a\\\::b#!b:c\:`)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parts, should.Match([]string{`a\:`, `b#!b`, `c:`}))
		})
		t.Run("Invalid", func(t *ftt.Test) {
			t.Run("Invalid rune", func(t *ftt.Test) {
				_, err := parseTestCaseName("aa\xff")
				assert.Loosely(t, err, should.ErrLike("invalid UTF-8 rune at byte 2"))
			})
			t.Run("Unicode replacement character", func(t *ftt.Test) {
				_, err := parseTestCaseName("aa\ufffd")
				assert.Loosely(t, err, should.ErrLike("invalid UTF-8 rune at byte 2"))
			})
			t.Run("Invalid escape sequence", func(t *ftt.Test) {
				_, err := parseTestCaseName(`\#`)
				assert.Loosely(t, err, should.ErrLike("got unexpected character '#' at byte 1, while processing escape sequence (\\); only the characters \\ and : may be escaped"))
			})
			t.Run("Unfinished escape sequence", func(t *ftt.Test) {
				_, err := parseTestCaseName(`a\`)
				assert.Loosely(t, err, should.ErrLike("unfinished escape sequence at byte 2, got end of string; expected one of \\ or :"))
			})
		})
	})
}

func FuzzTestIDRoundtrip(f *testing.F) {
	// Seeds
	f.Add("module", "scheme", "coarse", "fine", "case")
	f.Add("module", "scheme", "coarse", "fine", "case:subcase:subcase")
	f.Add("legacy", "legacy", "", "", "ninja://case:subcase:subcase")
	f.Add("\ufffd", "scheme", "coarse", "fine", "case")

	f.Fuzz(func(t *testing.T, moduleName string, moduleScheme string, coarseName string, fineName string, caseName string) {
		// Any test identifier that passes validation should roundtrip
		// back to the original.
		id := BaseTestIdentifier{
			ModuleName:   moduleName,
			ModuleScheme: moduleScheme,
			CoarseName:   coarseName,
			FineName:     fineName,
			CaseName:     caseName,
		}
		if err := ValidateBaseTestIdentifier(id); err != nil {
			// Invalid input.
			return
		}
		encoded := EncodeTestID(id)
		decoded, err := ParseAndValidateTestID(encoded)
		if err != nil {
			t.Errorf("Failed to parse input: %v", encoded)
		}
		assert.Loosely(t, decoded, should.Match(id))
	})
}

// These parsing, validation and encoding methods are called many billions of times per day.
// As such, every microsecond is equivalent to a few hours of CPU time per day and up to a
// few milliseconds per batch RPC.
//
// We benchmark them to ensure they are reasonably efficient.

// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
// BenchmarkValidateStructuredTestIdentifier-96    	  358879	      3212 ns/op	     337 B/op	      16 allocs/op
func BenchmarkValidateStructuredTestIdentifier(b *testing.B) {
	id := &pb.TestIdentifier{
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
		err := ValidateStructuredTestIdentifierForStorage(id)
		if err != nil {
			panic(err)
		}
	}
}

// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
// BenchmarkParseAndValidateTestIDLegacy-96    	  654147	      1883 ns/op	      48 B/op	       6 allocs/op
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
// BenchmarkParseAndValidateTestID-96    	  304288	      3681 ns/op	     273 B/op	      14 allocs/op
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
// BenchmarkEncode-96    	 1973002	       611.2 ns/op	     144 B/op	       1 allocs/op
func BenchmarkEncode(b *testing.B) {
	id := BaseTestIdentifier{
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
