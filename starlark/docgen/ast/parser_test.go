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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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

ellipsis = 1 + 2

alias1 = func1
alias2 = ext2.deeper.deep

decl = namespace.declare(
    skipped,
    arg1 = 123,
    arg2 = 1 + 1,
    arg3 = func1,
    arg4 = ext2.deeper.deep,
    arg5 = nested(nested_arg = 1),
)

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
  ellipsis = ...
  alias1 = func1
  alias2 = ext2.deeper.deep
  decl = namespace.declare(...)
    arg1 = 123
    arg2 = ...
    arg3 = func1
    arg4 = ext2.deeper.deep
    arg5 = nested(...)
      nested_arg = 1
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

	ftt.Run("Works", t, func(t *ftt.Test) {
		mod, err := ParseModule(&syntax.FileOptions{}, "mod.star", goodInput, func(s string) (string, error) { return s, nil })
		assert.Loosely(t, err, should.BeNil)
		buf := strings.Builder{}
		dumpTree(mod, &buf, "")
		assert.Loosely(t, buf.String(), should.Equal(expectedDumpOutput))

		t.Run("Docstrings extraction", func(t *ftt.Test) {
			assert.Loosely(t, mod.Doc(), should.Equal("Module doc string.\n\nMultiline.\n"))
			assert.Loosely(t,
				nodeByName(mod, "func1").Doc(), should.Equal(
					"Doc string.\n\n  Multiline.\n  ",
				))
		})

		t.Run("Spans extraction", func(t *ftt.Test) {
			l, r := nodeByName(mod, "func1").Span()
			assert.Loosely(t, l.Filename(), should.Equal("mod.star"))
			assert.Loosely(t, r.Filename(), should.Equal("mod.star"))

			// Spans the entire function definition.
			assert.Loosely(t, extractSpan(goodInput, l, r), should.Equal(`def func1(*, a, b, c=None, **kwargs):
  """Doc string.

  Multiline.
  """
  body`))
		})

		t.Run("Comments extraction", func(t *ftt.Test) {
			assert.Loosely(t, nodeByName(mod, "func1").Comments(), should.Equal(`Function comment.

  With indent.

More.`))

			strct := nodeByName(mod, "struct_stuff").(*Namespace)
			assert.Loosely(t, strct.Comments(), should.Equal("Struct comment."))

			// Individual struct entries are annotated with comments too.
			assert.Loosely(t, nodeByName(strct, "key1").Comments(), should.Equal("Key comment 1."))

			// Nested structs are also annotated.
			nested := nodeByName(strct, "nested").(*Namespace)
			assert.Loosely(t, nested.Comments(), should.Equal("Nested namespace."))

			// Keys do not pick up comments not intended for them.
			assert.Loosely(t, nodeByName(nested, "key1").Comments(), should.BeEmpty)

			// Top module comment is not extracted currently, it is relatively hard
			// to do. We have a docstring though, so it's not a big deal.
			assert.Loosely(t, mod.Comments(), should.BeEmpty)
		})
	})
}

func dumpTree(nd Node, w io.Writer, indent string) {
	// recurseInto is used to visit Namespace and Module.
	recurseInto := func(nodes []Node) {
		if len(nodes) != 0 {
			for _, n := range nodes {
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
	case *Invocation:
		fmt.Fprintf(w, "%s%s = %s(...)\n", indent, n.name, strings.Join(n.Func, "."))
		recurseInto(n.Args)
	case *Namespace:
		fmt.Fprintf(w, "%s%s = namespace\n", indent, n.name)
		recurseInto(n.Nodes)
	case *Module:
		fmt.Fprintf(w, "%s%s = module\n", indent, n.name)
		recurseInto(n.Nodes)
	default:
		panic(fmt.Sprintf("unknown node kind %T", nd))
	}
}

func nodeByName(n EnumerableNode, name string) Node {
	for _, node := range n.EnumNodes() {
		if node.Name() == name {
			return node
		}
	}
	return nil
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
