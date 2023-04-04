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

package docstring

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParse(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		out := Parse(`An ACL entry: assigns given role (or roles) to given individuals or groups.

  Specifying an empty ACL entry is allowed. It is ignored everywhere. Useful for
  things like:

      luci.project(...)


  Args:
    roles :   a single role (as acl.role) or a list of roles to assign,
        blah-blah multiline.

    groups: a single group name or a list of groups to assign the role to.
    stuff: line1
      line2
      line3
    users: a single user email or a list of emails to assign the role to.


    empty:


  Returns:
    acl.entry struct, consider it opaque.
    Multiline.

  Note:
    blah-blah.

  Empty:
`)

		So(out.Description, ShouldResemble, strings.Join([]string{
			"An ACL entry: assigns given role (or roles) to given individuals or groups.",
			"",
			"Specifying an empty ACL entry is allowed. It is ignored everywhere. Useful for",
			"things like:",
			"",
			"    luci.project(...)",
		}, "\n"))

		So(out.Fields, ShouldResemble, []FieldsBlock{
			{
				Title: "Args",
				Fields: []Field{
					{"roles", "a single role (as acl.role) or a list of roles to assign, blah-blah multiline."},
					{"groups", "a single group name or a list of groups to assign the role to."},
					{"stuff", "line1 line2 line3"},
					{"users", "a single user email or a list of emails to assign the role to."},
					{"empty", ""},
				},
			},
		})

		So(out.Remarks, ShouldResemble, []RemarkBlock{
			{"Returns", "acl.entry struct, consider it opaque.\nMultiline."},
			{"Note", "blah-blah."},
			{"Empty", ""},
		})
	})
}

func TestNormalizedLines(t *testing.T) {
	t.Parallel()

	Convey("Empty", t, func() {
		So(normalizedLines("  \n\n\t\t\n  "), ShouldHaveLength, 0)
	})

	Convey("One line and some space", t, func() {
		So(normalizedLines("  \n\n  Blah   \n\t\t\n  \n"), ShouldResemble, []string{"Blah"})
	})

	Convey("Deindents", t, func() {
		So(normalizedLines(`First paragraph,
		perhaps multiline.

		Second paragraph.

			Deeper indentation.

		Third paragraph.
		`), ShouldResemble,
			[]string{
				"First paragraph,",
				"perhaps multiline.",
				"",
				"Second paragraph.",
				"",
				"\tDeeper indentation.",
				"",
				"Third paragraph.",
			})
	})
}

func TestDeindent(t *testing.T) {
	t.Parallel()

	Convey("Space only", t, func() {
		So(
			deindent([]string{"  ", " \t\t  \t", ""}),
			ShouldResemble,
			[]string{"", "", ""},
		)
	})

	Convey("Nothing to deindent", t, func() {
		So(
			deindent([]string{"  ", "a  ", "b", "  "}),
			ShouldResemble,
			[]string{"", "a  ", "b", ""},
		)
	})

	Convey("Deindention works", t, func() {
		So(
			deindent([]string{"   ", "", "  a", "  b", "    c"}),
			ShouldResemble,
			[]string{"", "", "a", "b", "  c"},
		)
	})

	Convey("Works with tabs too", t, func() {
		So(
			deindent([]string{"\t\t", "", "\ta", "\tb", "\t\tc"}),
			ShouldResemble,
			[]string{"", "", "a", "b", "\tc"},
		)
	})
}
