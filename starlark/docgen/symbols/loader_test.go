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

package symbols

import (
	"fmt"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"io"
	"strings"
	"testing"
)

var srcs = map[string]string{
	"init.star": `
load("another.star", _mod="exported")
load("third.star", _third="exported")

broken = pub1  # not defined yet

pub1 = struct(
    sym = _mod.func,
    const = 123,
    broken1 = unknown,
    broken2 = _mod.unknown,
    broken3 = pub1,  # pub1 is assumed not defined yet
)
pub2 = _mod

def pub3():
  pass

pub4 = _third.deeper.deep

pub5 = _mod.deeper.deep(
    arg1 = 123,
    arg2 = pub4,
)

pub6 = pub5
`,

	"another.star": `
def _func():
  pass

exported = struct(
    func = _func,
    deeper = struct(deep = _func),
)

`,

	"third.star": `
load("another.star", _mod="exported")
exported = _mod
`,
}

const expectedInitStar = `init.star = *ast.Namespace at init.star:2:1 {
  _mod = *ast.Namespace at another.star:5:1 {
    func = *ast.Function _func at another.star:2:1
    deeper = *ast.Namespace at another.star:7:5 {
      deep = *ast.Function _func at another.star:2:1
    }
  }
  _third = *ast.Namespace at another.star:5:1 {
    func = *ast.Function _func at another.star:2:1
    deeper = *ast.Namespace at another.star:7:5 {
      deep = *ast.Function _func at another.star:2:1
    }
  }
  broken = <broken>
  pub1 = *ast.Namespace at init.star:7:1 {
    sym = *ast.Function _func at another.star:2:1
    const = *ast.Var const at init.star:9:5
    broken1 = <broken>
    broken2 = <broken>
    broken3 = <broken>
  }
  pub2 = *ast.Namespace at another.star:5:1 {
    func = *ast.Function _func at another.star:2:1
    deeper = *ast.Namespace at another.star:7:5 {
      deep = *ast.Function _func at another.star:2:1
    }
  }
  pub3 = *ast.Function pub3 at init.star:16:1
  pub4 = *ast.Function _func at another.star:2:1
  pub5 = *ast.Invocation of *symbols.Term deep at init.star:21:1 {
    arg1 = *ast.Var arg1 at init.star:22:5
    arg2 = *ast.Function _func at another.star:2:1
  }
  pub6 = *ast.Invocation of *symbols.Term deep at init.star:21:1 {
    arg1 = *ast.Var arg1 at init.star:22:5
    arg2 = *ast.Function _func at another.star:2:1
  }
}
`

const expectedThirdStar = `third.star = *ast.Namespace at third.star:2:1 {
  _mod = *ast.Namespace at another.star:5:1 {
    func = *ast.Function _func at another.star:2:1
    deeper = *ast.Namespace at another.star:7:5 {
      deep = *ast.Function _func at another.star:2:1
    }
  }
  exported = *ast.Namespace at another.star:5:1 {
    func = *ast.Function _func at another.star:2:1
    deeper = *ast.Namespace at another.star:7:5 {
      deep = *ast.Function _func at another.star:2:1
    }
  }
}
`

func TestLoader(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		l := Loader{
			Normalize: func(parent, module string) (string, error) { return module, nil },
			Source:    source(srcs),
		}

		init, err := l.Load("init.star")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, symbolToString(init), should.Equal(expectedInitStar))

		third, err := l.Load("third.star")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, symbolToString(third), should.Equal(expectedThirdStar))

		// 'another.star' is not reparsed. Both init and third eventually refer to
		// exact same AST nodes.
		s1 := Lookup(init, "pub1", "sym")
		s2 := Lookup(third, "exported", "func")
		assert.Loosely(t, s1.Def().Name(), should.Equal("_func")) // correct node
		assert.Loosely(t, s1.Def(), should.Equal(s2.Def()))       // exact same pointers
	})

	ftt.Run("Recursive deps", t, func(t *ftt.Test) {
		l := Loader{
			Normalize: func(parent, module string) (string, error) { return module, nil },
			Source: source(map[string]string{
				"a.star": `load("b.star", "_")`,
				"b.star": `load("a.star", "_")`,
			})}
		_, err := l.Load("a.star")
		assert.Loosely(t, err.Error(), should.Equal("in a.star: in b.star: in a.star: recursive dependency"))
	})
}

// source makes a source-provider function.
func source(src map[string]string) func(string) (string, error) {
	return func(module string) (string, error) {
		body, ok := src[module]
		if !ok {
			return "", fmt.Errorf("no such module")
		}
		return body, nil
	}
}

func symbolToString(s Symbol) string {
	buf := strings.Builder{}
	dumpTree(s, &buf, "")
	return buf.String()
}

func dumpTree(s Symbol, w io.Writer, indent string) {
	switch sym := s.(type) {
	case *BrokenSymbol:
		fmt.Fprintf(w, "%s%s = <broken>\n", indent, s.Name())
	case *Term:
		node := sym.Def()
		pos, _ := node.Span()
		fmt.Fprintf(w, "%s%s = %T %s at %s\n", indent, s.Name(), node, node.Name(), pos)
	case *Invocation:
		node := sym.Def()
		pos, _ := node.Span()
		fmt.Fprintf(w, "%s%s = %T of %T %s at %s {\n",
			indent, s.Name(), node, sym.fn, sym.fn.Name(), pos)
		for _, s := range sym.args {
			dumpTree(s, w, indent+"  ")
		}
		fmt.Fprintf(w, "%s}\n", indent)
	case *Struct:
		node := sym.Def()
		pos, _ := node.Span()
		fmt.Fprintf(w, "%s%s = %T at %s {\n", indent, s.Name(), node, pos)
		for _, s := range sym.symbols {
			dumpTree(s, w, indent+"  ")
		}
		fmt.Fprintf(w, "%s}\n", indent)
	}
}
