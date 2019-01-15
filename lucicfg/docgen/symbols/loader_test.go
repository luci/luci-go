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
	"io"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("Works", t, func() {
		l := Loader{Source: source(srcs)}

		init, err := l.Load("init.star")
		So(err, ShouldBeNil)
		So(symbolToString(init), ShouldEqual, expectedInitStar)

		third, err := l.Load("third.star")
		So(err, ShouldBeNil)
		So(symbolToString(third), ShouldEqual, expectedThirdStar)

		// 'another.star' is not reparsed. Both init and third eventually refer to
		// exact same AST nodes.
		s1 := Lookup(init, "pub1", "sym")
		s2 := Lookup(third, "exported", "func")
		So(s1.Def().Name(), ShouldEqual, "_func") // correct node
		So(s1.Def(), ShouldEqual, s2.Def())       // exact same pointers
	})

	Convey("Recursive deps", t, func() {
		l := Loader{Source: source(map[string]string{
			"a.star": `load("b.star", "_")`,
			"b.star": `load("a.star", "_")`,
		})}
		_, err := l.Load("a.star")
		So(err.Error(), ShouldEqual, "in a.star: in b.star: in a.star: recursive dependency")
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
