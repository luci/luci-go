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

package git

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/data/strpair"
)

func TestNormalizeKey(t *testing.T) {
	t.Parallel()

	Convey("normalize key", t, func() {
		Convey("removes surrounding white space", func() {
			So(NormalizeKey("   R   "), ShouldEqual, "R")
		})

		Convey("converts to title case", func() {
			So(NormalizeKey("BUG"), ShouldEqual, "Bug")
			So(NormalizeKey("GIT-FOOTER"), ShouldEqual, "Git-Footer")
			So(NormalizeKey("GiT-fOoTeR"), ShouldEqual, "Git-Footer")
			So(NormalizeKey("123-_ABC"), ShouldEqual, "123-_abc")
		})
	})
}

func TestParseFooter(t *testing.T) {
	t.Parallel()

	parse := func(line, expectedKey, expectedValue string) {
		actualKey, actualValue := ParseFooter(line)
		So(actualKey, ShouldEqual, expectedKey)
		So(actualValue, ShouldEqual, expectedValue)
	}

	Convey("parse footer", t, func() {
		Convey("valid", func() {
			parse("GIT-FOOTER: 12345", "Git-Footer", "12345")
			parse("GIT-FOOTER: here could be anything",
				"Git-Footer", "here could be anything")
			parse("    GIT-FOOTER:     whitespace     ", "Git-Footer", "whitespace")
		})
		Convey("invalid", func() {
			testInvalid := func(line string) {
				parse(line, "", "")
			}
			testInvalid("NOT VALID")
			testInvalid("GIT-FOOTER=12345")
			testInvalid("GIT-FOOTER  :  12345")    // additional whitespace
			testInvalid("GIT|FOOTER: 12345")       // invalid key
			testInvalid("http://www.google.com/")  // in ignore list
			testInvalid("https://www.google.com/") // in ignore list
		})
	})
}

func TestParseFooterLines(t *testing.T) {
	t.Parallel()

	Convey("parse footer lines", t, func() {
		Convey("simple", func() {
			actual := ParseFooterLines([]string{
				"Git-Foo: foo",
				"Git-Bar: bar",
			})
			So(actual, ShouldResemble, strpair.ParseMap([]string{
				"Git-Foo:foo",
				"Git-Bar:bar",
			}))
		})
		Convey("omit malformed lines", func() {
			actual := ParseFooterLines([]string{
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

		Convey("latter footer takes precedence", func() {
			actual := ParseFooterLines([]string{
				"Git-Footer:foo",
				"Git-Footer:bar",
				"Git-Footer:baz",
			})
			So(actual["Git-Footer"], ShouldResemble, []string{"baz", "bar", "foo"})
		})
	})
}

func TestSplitFooters(t *testing.T) {
	t.Parallel()

	assertFooterLines := func(message string, splitLineNum int) {
		nonFooterLines, footerLines := SplitFooterLines(message)
		messageLines := strings.Split(message, "\n")

		So(nonFooterLines, ShouldResemble, messageLines[:splitLineNum])
		So(footerLines, ShouldResemble, messageLines[splitLineNum:])
	}

	Convey("split footers", t, func() {
		Convey("simple", func() {
			message := `commit message

commit details...

Git-Footer: 12345
Git-Footer: 23456`
			assertFooterLines(message, 4)
		})

		Convey("footer contains malformed lines", func() {
			message := `commit message

commit details...

Git-Footer: 12345
Random String in between
Git-Footer: 23456
Random Stuff at the end`
			assertFooterLines(message, 4)
		})

		Convey("whole message is footer", func() {
			message := `Git-Footer: 12345
Git-Footer: 23456`
			nonFooterLines, footerLines := SplitFooterLines(message)
			So(footerLines, ShouldBeNil)
			So(nonFooterLines, ShouldResemble, strings.Split(message, "\n"))
		})

		Convey("footer line starts with malformed lines", func() {
			message := `commit message

commit details...

This line should be non-footer line
Git-Footer: 12345
Git-Footer: 23456`
			nonFooterLines, footerLines := SplitFooterLines(message)
			messageLines := strings.Split(message, "\n")

			So(footerLines, ShouldResemble, messageLines[len(messageLines)-2:])
			expectedNonFooterLines := append([]string(nil), messageLines[:len(messageLines)-2]...)
			expectedNonFooterLines = append(expectedNonFooterLines, "")
			So(nonFooterLines, ShouldResemble, expectedNonFooterLines)
		})

		Convey("trim leading and trailing new line ", func() {
			message := `

commit message

Git-Footer: 12345


`
			nonFooterLines, footerLines := SplitFooterLines(message)
			So(nonFooterLines, ShouldResemble, []string{"commit message", ""})
			So(footerLines, ShouldResemble, []string{"Git-Footer: 12345"})
		})
	})
}
