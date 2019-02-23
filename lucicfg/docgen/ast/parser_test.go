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

package ast

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"go.starlark.net/syntax"

	. "github.com/smartystreets/goconvey/convey"
)

const goodInput = `
# Copyright comment or something.

"""Module doc string.

Multiline.
"""

# Load comment.
load('@stdlib//another.star', 'ext1', ext2='ext_name')

# Skipped comment.

# Function comment.
#
#   With indent.
#
# More.
def func1(*, a, b, c=None, **kwargs):
  """Doc string.

  Multiline.
  """
  body

# No docstring
def func2():
  pass

# Weird doc string: number instead of a string.
def func3():
  42

const_int = 123
const_str = 'str'

ellipsis1 = some_unrecognized_call()
ellipsis2 = 1 + 2
ellipsis3 = a.b(c)

alias1 = func1
alias2 = ext2.deeper.deep

# Struct comment.
struct_stuff = struct(
    const = 123,
    stuff = 1+2+3,
    # Key comment 1.
    key1 = func1,
    key2 = ext2.deeper.deep,
    # Nested namespace.
    nested = struct(key1=v1, key2=v2),

    **skipped
)

skipped, skipped = a, b
skipped += 123
`

const expectedDumpOutput = `mod.star = module
  ext1 -> ext1 in @stdlib//another.star
  ext2 -> ext_name in @stdlib//another.star
  func1 = func
  func2 = func
  func3 = func
  const_int = 123
  const_str = str
  ellipsis1 = ...
  ellipsis2 = ...
  ellipsis3 = ...
  alias1 = func1
  alias2 = ext2.deeper.deep
  struct_stuff = namespace
    const = 123
    stuff = ...
    key1 = func1
    key2 = ext2.deeper.deep
    nested = namespace
      key1 = v1
      key2 = v2
`

func TestParseModule(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		mod, err := ParseModule("mod.star", goodInput)
		So(err, ShouldBeNil)
		buf := strings.Builder{}
		dumpTree(mod, &buf, "")
		So(buf.String(), ShouldEqual, expectedDumpOutput)

		Convey("Docstrings extraction", func() {
			So(mod.Doc(), ShouldEqual, "Module doc string.\n\nMultiline.\n")
			So(
				mod.NodeByName("func1").Doc(), ShouldEqual,
				"Doc string.\n\n  Multiline.\n  ",
			)
		})

		Convey("Spans extraction", func() {
			l, r := mod.NodeByName("func1").Span()
			So(l.Filename(), ShouldEqual, "mod.star")
			So(r.Filename(), ShouldEqual, "mod.star")

			// Spans the entire function definition.
			So(extractSpan(goodInput, l, r), ShouldEqual, `def func1(*, a, b, c=None, **kwargs):
  """Doc string.

  Multiline.
  """
  body`)
		})

		Convey("Comments extraction", func() {
			So(mod.NodeByName("func1").Comments(), ShouldEqual, `Function comment.

  With indent.

More.`)

			strct := mod.NodeByName("struct_stuff").(*Namespace)
			So(strct.Comments(), ShouldEqual, "Struct comment.")

			// Individual struct entries are annotated with comments too.
			So(strct.NodeByName("key1").Comments(), ShouldEqual, "Key comment 1.")

			// Nested structs are also annotated.
			nested := strct.NodeByName("nested").(*Namespace)
			So(nested.Comments(), ShouldEqual, "Nested namespace.")

			// Keys do not pick up comments not intended for them.
			So(nested.NodeByName("key1").Comments(), ShouldEqual, "")

			// Top module comment is not extracted currently, it is relatively hard
			// to do. We have a docstring though, so it's not a big deal.
			So(mod.Comments(), ShouldEqual, "")
		})
	})
}

func dumpTree(nd Node, w io.Writer, indent string) {
	// recurseInto is used to visit Namespace and Module.
	recurseInto := func(kind string, n *Namespace) {
		fmt.Fprintf(w, "%s%s = %s\n", indent, n.name, kind)
		if len(n.Nodes) != 0 {
			for _, n := range n.Nodes {
				dumpTree(n, w, indent+"  ")
			}
		} else {
			fmt.Fprintf(w, "%s  <empty>\n", indent)
		}
	}

	switch n := nd.(type) {
	case *Var:
		fmt.Fprintf(w, "%s%s = %v\n", indent, n.name, n.Value)
	case *Function:
		fmt.Fprintf(w, "%s%s = func\n", indent, n.name)
	case *Reference:
		fmt.Fprintf(w, "%s%s = %s\n", indent, n.name, strings.Join(n.Path, "."))
	case *ExternalReference:
		fmt.Fprintf(w, "%s%s -> %s in %s\n", indent, n.name, n.ExternalName, n.Module)
	case *Namespace:
		recurseInto("namespace", n)
	case *Module:
		recurseInto("module", &n.Namespace)
	default:
		panic(fmt.Sprintf("unknown node kind %T", nd))
	}
}

func extractSpan(body string, start, end syntax.Position) string {
	lines := strings.Split(body, "\n")

	// Note: sloppy, but good enough for the test. Also note that Line and Col
	// are 1-based indexes.
	var out []string
	out = append(out, lines[start.Line-1][start.Col-1:])
	out = append(out, lines[start.Line:end.Line-1]...)
	out = append(out, lines[end.Line-1][:end.Col-1])

	return strings.Join(out, "\n")
}
