// Copyright 2020 The LUCI Authors.
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

package footer

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNormalizeKey(t *testing.T) {
	t.Parallel()

	ftt.Run("NormalizeKey", t, func(t *ftt.Test) {
		t.Run("Remove surrounding whitespace", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeKey("   R   "), should.Equal("R"))
		})

		t.Run("Convert key to title case", func(t *ftt.Test) {
			assert.Loosely(t, NormalizeKey("BUG"), should.Equal("Bug"))
			assert.Loosely(t, NormalizeKey("GIT-FOOTER"), should.Equal("Git-Footer"))
			assert.Loosely(t, NormalizeKey("GiT-fOoTeR"), should.Equal("Git-Footer"))
			assert.Loosely(t, NormalizeKey("123-_ABC"), should.Equal("123-_abc"))
		})
	})
}

func TestParseLine(t *testing.T) {
	t.Parallel()

	parse := func(line, expectedKey, expectedValue string) {
		actualKey, actualValue := ParseLine(line)
		assert.Loosely(t, actualKey, should.Equal(expectedKey))
		assert.Loosely(t, actualValue, should.Equal(expectedValue))
	}

	ftt.Run("ParseLine", t, func(t *ftt.Test) {
		t.Run("Parse valid footer line", func(t *ftt.Test) {
			parse("GIT-FOOTER: 12345", "Git-Footer", "12345")
			parse("GIT-FOOTER: here could be anything",
				"Git-Footer", "here could be anything")
			parse("    GIT-FOOTER:     whitespace     ", "Git-Footer", "whitespace")
		})
		t.Run("Parse invalid footer line", func(t *ftt.Test) {
			parseInvalid := func(line string) {
				parse(line, "", "")
			}
			parseInvalid("NOT VALID")
			parseInvalid("GIT-FOOTER=12345")
			parseInvalid("GIT-FOOTER  :  12345")    // additional whitespace
			parseInvalid("GIT|FOOTER: 12345")       // invalid key
			parseInvalid("http://www.google.com/")  // in ignore list
			parseInvalid("https://www.google.com/") // in ignore list
		})
	})
}

func TestParseLines(t *testing.T) {
	t.Parallel()

	ftt.Run("ParseLines", t, func(t *ftt.Test) {
		t.Run("Simple", func(t *ftt.Test) {
			actual := ParseLines([]string{
				"Git-Foo: foo",
				"Git-Bar: bar",
			})
			assert.Loosely(t, actual, should.Resemble(strpair.ParseMap([]string{
				"Git-Foo:foo",
				"Git-Bar:bar",
			})))
		})
		t.Run("Omit malformed lines", func(t *ftt.Test) {
			actual := ParseLines([]string{
				"Git-Foo: foo",
				"Random string in the middle",
				"Git-Bar: bar",
				"Random string at the end",
			})
			assert.Loosely(t, actual, should.Resemble(strpair.ParseMap([]string{
				"Git-Foo:foo",
				"Git-Bar:bar",
			})))
		})

		t.Run("Honer footer ordering", func(t *ftt.Test) {
			actual := ParseLines([]string{
				"Git-Footer:foo",
				"Git-Footer:bar",
				"Git-Footer:baz",
			})
			assert.Loosely(t, actual["Git-Footer"], should.Resemble([]string{"baz", "bar", "foo"}))
		})
	})
}

func TestSplitLines(t *testing.T) {
	t.Parallel()

	assertSplitAt := func(message string, splitLineNum int) {
		nonFooterLines, footerLines := SplitLines(message)
		messageLines := strings.Split(message, "\n")

		assert.Loosely(t, nonFooterLines, should.Resemble(messageLines[:splitLineNum]))
		assert.Loosely(t, footerLines, should.Resemble(messageLines[splitLineNum:]))
	}

	ftt.Run("SplitLines", t, func(t *ftt.Test) {
		t.Run("Simple", func(t *ftt.Test) {
			message := `commit message

commit details...

Git-Footer: 12345
Git-Footer: 23456`
			assertSplitAt(message, 4)
		})

		t.Run("Include malformed lines in last paragraph", func(t *ftt.Test) {
			message := `commit message

commit details...

Git-Footer: 12345
Random String in between
Git-Footer: 23456
Random Stuff at the end`
			assertSplitAt(message, 4)
		})

		t.Run("Does not split when whole message is footer", func(t *ftt.Test) {
			message := `Git-Footer: 12345
Git-Footer: 23456`
			nonFooterLines, footerLines := SplitLines(message)
			assert.Loosely(t, footerLines, should.BeNil)
			assert.Loosely(t, nonFooterLines, should.Resemble(strings.Split(message, "\n")))
		})

		t.Run("Splits last paragraph when it starts with text", func(t *ftt.Test) {
			message := `commit message

commit details...

This line should be non-footer line
Git-Footer: 12345
Git-Footer: 23456`
			nonFooterLines, footerLines := SplitLines(message)
			messageLines := strings.Split(message, "\n")

			assert.Loosely(t, footerLines, should.Resemble(messageLines[len(messageLines)-2:]))
			expectedNonFooterLines := append([]string(nil), messageLines[:len(messageLines)-2]...)
			expectedNonFooterLines = append(expectedNonFooterLines, "")
			assert.Loosely(t, nonFooterLines, should.Resemble(expectedNonFooterLines))
		})

		t.Run("Trims leading and trailing new lines", func(t *ftt.Test) {
			message := `

commit message

Git-Footer: 12345


`
			nonFooterLines, footerLines := SplitLines(message)
			assert.Loosely(t, nonFooterLines, should.Resemble([]string{"commit message", ""}))
			assert.Loosely(t, footerLines, should.Resemble([]string{"Git-Footer: 12345"}))
		})
	})
}
