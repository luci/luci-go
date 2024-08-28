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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
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

		assert.Loosely(t, out.Description, should.Resemble(strings.Join([]string{
			"An ACL entry: assigns given role (or roles) to given individuals or groups.",
			"",
			"Specifying an empty ACL entry is allowed. It is ignored everywhere. Useful for",
			"things like:",
			"",
			"    luci.project(...)",
		}, "\n")))

		assert.Loosely(t, out.Fields, should.Resemble([]FieldsBlock{
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
		}))

		assert.Loosely(t, out.Remarks, should.Resemble([]RemarkBlock{
			{"Returns", "acl.entry struct, consider it opaque.\nMultiline."},
			{"Note", "blah-blah."},
			{"Empty", ""},
		}))
	})
}

func TestNormalizedLines(t *testing.T) {
	t.Parallel()

	ftt.Run("Empty", t, func(t *ftt.Test) {
		assert.Loosely(t, normalizedLines("  \n\n\t\t\n  "), should.HaveLength(0))
	})

	ftt.Run("One line and some space", t, func(t *ftt.Test) {
		assert.Loosely(t, normalizedLines("  \n\n  Blah   \n\t\t\n  \n"), should.Resemble([]string{"Blah"}))
	})

	ftt.Run("Deindents", t, func(t *ftt.Test) {
		assert.Loosely(t, normalizedLines(`First paragraph,
		perhaps multiline.

		Second paragraph.

			Deeper indentation.

		Third paragraph.
		`), should.Resemble(
			[]string{
				"First paragraph,",
				"perhaps multiline.",
				"",
				"Second paragraph.",
				"",
				"\tDeeper indentation.",
				"",
				"Third paragraph.",
			}))
	})
}

func TestDeindent(t *testing.T) {
	t.Parallel()

	ftt.Run("Space only", t, func(t *ftt.Test) {
		assert.Loosely(t,
			deindent([]string{"  ", " \t\t  \t", ""}),
			should.Resemble(
				[]string{"", "", ""},
			))
	})

	ftt.Run("Nothing to deindent", t, func(t *ftt.Test) {
		assert.Loosely(t,
			deindent([]string{"  ", "a  ", "b", "  "}),
			should.Resemble(
				[]string{"", "a  ", "b", ""},
			))
	})

	ftt.Run("Deindention works", t, func(t *ftt.Test) {
		assert.Loosely(t,
			deindent([]string{"   ", "", "  a", "  b", "    c"}),
			should.Resemble(
				[]string{"", "", "a", "b", "  c"},
			))
	})

	ftt.Run("Works with tabs too", t, func(t *ftt.Test) {
		assert.Loosely(t,
			deindent([]string{"\t\t", "", "\ta", "\tb", "\t\tc"}),
			should.Resemble(
				[]string{"", "", "a", "b", "\tc"},
			))
	})
}
