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
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNormalizeKey(t *testing.T) {
	t.Parallel()

	t.Run("NormalizeKey", func(t *testing.T) {
		t.Run("Remove surrounding whitespace", func(t *testing.T) {
			assert.That(t, NormalizeKey("   R   "), should.Equal("R"))
		})

		t.Run("Convert key to title case", func(t *testing.T) {
			assert.That(t, NormalizeKey("BUG"), should.Equal("Bug"))
			assert.That(t, NormalizeKey("GIT-FOOTER"), should.Equal("Git-Footer"))
			assert.That(t, NormalizeKey("GiT-fOoTeR"), should.Equal("Git-Footer"))
			assert.That(t, NormalizeKey("123-x_ABC"), should.Equal("123-X_abc"))
		})
	})
}

func TestParseLine(t *testing.T) {
	t.Parallel()

	parse := func(line, expectedKey, expectedValue string) {
		actualKey, actualValue := ParseLine(line)
		assert.That(t, actualKey, should.Equal(expectedKey))
		assert.That(t, actualValue, should.Equal(expectedValue))
	}

	t.Run("ParseLine", func(t *testing.T) {
		t.Run("Parse valid footer line", func(t *testing.T) {
			parse("GIT-FOOTER: 12345", "Git-Footer", "12345")
			parse("GIT-FOOTER: here could be anything",
				"Git-Footer", "here could be anything")
			parse("    GIT-FOOTER:     whitespace     ", "Git-Footer", "whitespace")
		})
		t.Run("Parse invalid footer line", func(t *testing.T) {
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

	t.Run("ParseLines", func(t *testing.T) {
		t.Run("Simple", func(t *testing.T) {
			actual := ParseLines([]string{
				"Git-Foo: foo",
				"Git-Bar: bar",
			})
			assert.That(t, actual, should.Match(strpair.ParseMap([]string{
				"Git-Foo:foo",
				"Git-Bar:bar",
			})))
		})
		t.Run("Omit malformed lines", func(t *testing.T) {
			actual := ParseLines([]string{
				"Git-Foo: foo",
				"Random string in the middle",
				"Git-Bar: bar",
				"Random string at the end",
			})
			assert.That(t, actual, should.Match(strpair.ParseMap([]string{
				"Git-Foo:foo",
				"Git-Bar:bar",
			})))
		})

		t.Run("Honer footer ordering", func(t *testing.T) {
			actual := ParseLines([]string{
				"Git-Footer:foo",
				"Git-Footer:bar",
				"Git-Footer:baz",
			})
			assert.That(t, actual["Git-Footer"], should.Match([]string{"baz", "bar", "foo"}))
		})
	})
}

func TestSplitLines(t *testing.T) {
	t.Parallel()

	assertSplitAt := func(message string, splitLineNum int) {
		nonFooterLines, footerLines := SplitLines(message)
		messageLines := strings.Split(message, "\n")

		assert.That(t, nonFooterLines, should.Match(messageLines[:splitLineNum]))
		assert.That(t, footerLines, should.Match(messageLines[splitLineNum:]))
	}

	t.Run("SplitLines", func(t *testing.T) {
		t.Run("Simple", func(t *testing.T) {
			message := `commit message

commit details...

Git-Footer: 12345
Git-Footer: 23456`
			assertSplitAt(message, 4)
		})

		t.Run("Include malformed lines in last paragraph", func(t *testing.T) {
			message := `commit message

commit details...

Git-Footer: 12345
Random String in between
Git-Footer: 23456
Random Stuff at the end`
			assertSplitAt(message, 4)
		})

		t.Run("Does not split when whole message is footer", func(t *testing.T) {
			message := `Git-Footer: 12345
Git-Footer: 23456`
			nonFooterLines, footerLines := SplitLines(message)
			assert.Loosely(t, footerLines, should.BeNil)
			assert.That(t, nonFooterLines, should.Match(strings.Split(message, "\n")))
		})

		t.Run("Splits last paragraph when it starts with text", func(t *testing.T) {
			message := `commit message

commit details...

This line should be non-footer line
Git-Footer: 12345
Git-Footer: 23456`
			nonFooterLines, footerLines := SplitLines(message)
			messageLines := strings.Split(message, "\n")

			assert.That(t, footerLines, should.Match(messageLines[len(messageLines)-2:]))
			expectedNonFooterLines := append([]string(nil), messageLines[:len(messageLines)-2]...)
			expectedNonFooterLines = append(expectedNonFooterLines, "")
			assert.That(t, nonFooterLines, should.Match(expectedNonFooterLines))
		})

		t.Run("Trims leading and trailing new lines", func(t *testing.T) {
			message := `

commit message

Git-Footer: 12345


`
			nonFooterLines, footerLines := SplitLines(message)
			assert.That(t, nonFooterLines, should.Match([]string{"commit message", ""}))
			assert.That(t, footerLines, should.Match([]string{"Git-Footer: 12345"}))
		})
	})
}
