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

	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalizeKey(t *testing.T) {
	t.Parallel()

	Convey("NormalizeKey", t, func() {
		Convey("Remove surrounding whitespace", func() {
			So(NormalizeKey("   R   "), ShouldEqual, "R")
		})

		Convey("Convert key to title case", func() {
			So(NormalizeKey("BUG"), ShouldEqual, "Bug")
			So(NormalizeKey("GIT-FOOTER"), ShouldEqual, "Git-Footer")
			So(NormalizeKey("GiT-fOoTeR"), ShouldEqual, "Git-Footer")
			So(NormalizeKey("123-_ABC"), ShouldEqual, "123-_abc")
		})
	})
}

func TestParseLine(t *testing.T) {
	t.Parallel()

	parse := func(line, expectedKey, expectedValue string) {
		actualKey, actualValue := ParseLine(line)
		So(actualKey, ShouldEqual, expectedKey)
		So(actualValue, ShouldEqual, expectedValue)
	}

	Convey("ParseLine", t, func() {
		Convey("Parse valid footer line", func() {
			parse("GIT-FOOTER: 12345", "Git-Footer", "12345")
			parse("GIT-FOOTER: here could be anything",
				"Git-Footer", "here could be anything")
			parse("    GIT-FOOTER:     whitespace     ", "Git-Footer", "whitespace")
		})
		Convey("Parse invalid footer line", func() {
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

	Convey("ParseLines", t, func() {
		Convey("Simple", func() {
			actual := ParseLines([]string{
				"Git-Foo: foo",
				"Git-Bar: bar",
			})
			So(actual, ShouldResemble, strpair.ParseMap([]string{
				"Git-Foo:foo",
				"Git-Bar:bar",
			}))
		})
		Convey("Omit malformed lines", func() {
			actual := ParseLines([]string{
				"Git-Foo: foo",
				"Random string in the middle",
				"Git-Bar: bar",
				"Random string at the end",
			})
			So(actual, ShouldResemble, strpair.ParseMap([]string{
				"Git-Foo:foo",
				"Git-Bar:bar",
			}))
		})

		Convey("Honer footer ordering", func() {
			actual := ParseLines([]string{
				"Git-Footer:foo",
				"Git-Footer:bar",
				"Git-Footer:baz",
			})
			So(actual["Git-Footer"], ShouldResemble, []string{"baz", "bar", "foo"})
		})
	})
}

func TestSplitLines(t *testing.T) {
	t.Parallel()

	assertSplitAt := func(message string, splitLineNum int) {
		nonFooterLines, footerLines := SplitLines(message)
		messageLines := strings.Split(message, "\n")

		So(nonFooterLines, ShouldResemble, messageLines[:splitLineNum])
		So(footerLines, ShouldResemble, messageLines[splitLineNum:])
	}

	Convey("SplitLines", t, func() {
		Convey("Simple", func() {
			message := `commit message

commit details...

Git-Footer: 12345
Git-Footer: 23456`
			assertSplitAt(message, 4)
		})

		Convey("Include malformed lines in last paragraph", func() {
			message := `commit message

commit details...

Git-Footer: 12345
Random String in between
Git-Footer: 23456
Random Stuff at the end`
			assertSplitAt(message, 4)
		})

		Convey("Does not split when whole message is footer", func() {
			message := `Git-Footer: 12345
Git-Footer: 23456`
			nonFooterLines, footerLines := SplitLines(message)
			So(footerLines, ShouldBeNil)
			So(nonFooterLines, ShouldResemble, strings.Split(message, "\n"))
		})

		Convey("Splits last paragraph when it starts with text", func() {
			message := `commit message

commit details...

This line should be non-footer line
Git-Footer: 12345
Git-Footer: 23456`
			nonFooterLines, footerLines := SplitLines(message)
			messageLines := strings.Split(message, "\n")

			So(footerLines, ShouldResemble, messageLines[len(messageLines)-2:])
			expectedNonFooterLines := append([]string(nil), messageLines[:len(messageLines)-2]...)
			expectedNonFooterLines = append(expectedNonFooterLines, "")
			So(nonFooterLines, ShouldResemble, expectedNonFooterLines)
		})

		Convey("Trims leading and trailing new lines", func() {
			message := `

commit message

Git-Footer: 12345


`
			nonFooterLines, footerLines := SplitLines(message)
			So(nonFooterLines, ShouldResemble, []string{"commit message", ""})
			So(footerLines, ShouldResemble, []string{"Git-Footer: 12345"})
		})
	})
}
