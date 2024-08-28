// Copyright 2018 The LUCI Authors.
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

package interpreter

import (
	"context"
	"fmt"
	"testing"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMakeModuleKey(t *testing.T) {
	t.Parallel()

	th := &starlark.Thread{}
	th.SetLocal(threadModKey, ModuleKey{"cur_pkg", "dir/cur.star"})

	ftt.Run("Works", t, func(t *ftt.Test) {
		k, err := MakeModuleKey(th, "//some/mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Resemble(ModuleKey{"cur_pkg", "some/mod"}))

		k, err = MakeModuleKey(th, "//some/mod/../blah")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Resemble(ModuleKey{"cur_pkg", "some/blah"}))

		k, err = MakeModuleKey(th, "some/mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Resemble(ModuleKey{"cur_pkg", "dir/some/mod"}))

		k, err = MakeModuleKey(th, "./mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Resemble(ModuleKey{"cur_pkg", "dir/mod"}))

		k, err = MakeModuleKey(th, "../mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Resemble(ModuleKey{"cur_pkg", "mod"}))

		// For absolute paths the thread is optional.
		k, err = MakeModuleKey(nil, "@pkg//some/mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Resemble(ModuleKey{"pkg", "some/mod"}))
	})

	ftt.Run("Fails", t, func(t *ftt.Test) {
		_, err := MakeModuleKey(th, "@//mod")
		assert.Loosely(t, err, should.NotBeNil)

		_, err = MakeModuleKey(th, "@mod")
		assert.Loosely(t, err, should.NotBeNil)

		// Imports outside of the package root are forbidden.
		_, err = MakeModuleKey(th, "//..")
		assert.Loosely(t, err, should.NotBeNil)

		_, err = MakeModuleKey(th, "../../mod")
		assert.Loosely(t, err, should.NotBeNil)

		// If the thread is given, it must have the package name.
		_, err = MakeModuleKey(&starlark.Thread{}, "//some/mod")
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestInterpreter(t *testing.T) {
	t.Parallel()

	ftt.Run("Stdlib scripts can load each other", t, func(t *ftt.Test) {
		keys, logs, err := runIntr(intrParams{
			stdlib: map[string]string{
				"builtins.star": `
					load("//loaded.star", "loaded_sym")
					exported_sym = "exported_sym_val"
					reimported_sym = loaded_sym
				`,
				"loaded.star": `loaded_sym = "loaded_sym_val"`,
			},
			scripts: map[string]string{
				"main.star": `print(reimported_sym, exported_sym)`,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, keys, should.HaveLength(0)) // main.star doesn't export anything itself
		assert.Loosely(t, logs, should.Resemble([]string{"[//main.star:1] loaded_sym_val exported_sym_val"}))
	})

	ftt.Run("User scripts can load each other and stdlib scripts", t, func(t *ftt.Test) {
		keys, _, err := runIntr(intrParams{
			stdlib: map[string]string{
				"lib.star": `lib_sym = True`,
			},
			scripts: map[string]string{
				"main.star": `
					load("//sub/loaded.star", _loaded_sym="loaded_sym")
					load("@stdlib//lib.star", _lib_sym="lib_sym")
					main_sym = True
					loaded_sym = _loaded_sym
					lib_sym = _lib_sym
				`,
				"sub/loaded.star": `loaded_sym = True`,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, keys, should.Resemble([]string{
			"lib_sym",
			"loaded_sym",
			"main_sym",
		}))
	})

	ftt.Run("Missing module", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//some.star", "some")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`cannot load //some.star: no such module`))
	})

	ftt.Run("Missing package", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("@pkg//some.star", "some")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`cannot load @pkg//some.star: no such package`))
	})

	ftt.Run("Malformed module reference", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("@@", "some")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`cannot load @@: a module path should be either '//<path>', '<path>' or '@<package>//<path>'`))
	})

	ftt.Run("Double dot module reference", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("../some.star", "some")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`cannot load ../some.star: outside the package root`))
	})

	ftt.Run("Predeclared are exposed to stdlib and user scripts", t, func(t *ftt.Test) {
		_, logs, err := runIntr(intrParams{
			predeclared: starlark.StringDict{
				"imported_sym": starlark.MakeInt(123),
			},
			stdlib: map[string]string{
				"builtins.star": `print(imported_sym)`,
			},
			scripts: map[string]string{
				"main.star": `print(imported_sym)`,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, logs, should.Resemble([]string{
			"[@stdlib//builtins.star:1] 123",
			"[//main.star:1] 123",
		}))
	})

	ftt.Run("Predeclared can access the context", t, func(t *ftt.Test) {
		fromCtx := ""
		type key struct{}
		_, _, err := runIntr(intrParams{
			ctx: context.WithValue(context.Background(), key{}, "ctx value"),
			predeclared: starlark.StringDict{
				"call_me": starlark.NewBuiltin("call_me", func(th *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
					fromCtx = Context(th).Value(key{}).(string)
					return starlark.None, nil
				}),
			},
			scripts: map[string]string{
				"main.star": `call_me()`,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fromCtx, should.Equal("ctx value"))
	})

	ftt.Run("Modules are loaded only once", t, func(t *ftt.Test) {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					load("//mod.star", "a")
					load("//mod.star", "b")

					print(a, b)
				`,
				"mod.star": `
					print("Loading")

					a = 1
					b = 2
				`,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, logs, should.Resemble([]string{
			"[//mod.star:2] Loading", // only once
			"[//main.star:5] 1 2",
		}))
	})

	ftt.Run("Module cycles are caught", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//mod1.star", "a")`,
				"mod1.star": `load("//mod2.star", "a")`,
				"mod2.star": `load("//mod1.star", "a")`,
			},
		})
		assert.Loosely(t, normalizeErr(err), should.Equal(`Traceback (most recent call last):
  //main.star: in <toplevel>
Error: cannot load //mod1.star: Traceback (most recent call last):
  //mod1.star: in <toplevel>
Error: cannot load //mod2.star: Traceback (most recent call last):
  //mod2.star: in <toplevel>
Error: cannot load //mod1.star: cycle in the module dependency graph`))
	})

	ftt.Run("Error in loaded module", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//mod.star", "z")`,
				"mod.star": `
					def f():
						boom = None()
					f()
				`,
			},
		})
		assert.Loosely(t, normalizeErr(err), should.Equal(`Traceback (most recent call last):
  //main.star: in <toplevel>
Error: cannot load //mod.star: Traceback (most recent call last):
  //mod.star: in <toplevel>
  //mod.star: in f
Error: invalid call of non-function (NoneType)`))
	})

	ftt.Run("Exec works", t, func(t *ftt.Test) {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					res = exec("//execed.star")
					print(res.a)
				`,

				"execed.star": `
					print('hi')
					a = 123
				`,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, logs, should.Resemble([]string{
			"[//execed.star:2] hi",
			"[//main.star:3] 123",
		}))
	})

	ftt.Run("Exec using relative path", t, func(t *ftt.Test) {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star":  `exec("//sub/1.star")`,
				"sub/1.star": `exec("./2.star")`,
				"sub/2.star": `print('hi')`,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, logs, should.Resemble([]string{
			"[//sub/2.star:1] hi",
		}))
	})

	ftt.Run("Exec into another package", t, func(t *ftt.Test) {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `exec("@stdlib//exec1.star")`,
			},
			stdlib: map[string]string{
				"exec1.star": `exec("//exec2.star")`,
				"exec2.star": `print("hi")`,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, logs, should.Resemble([]string{
			"[@stdlib//exec2.star:1] hi",
		}))
	})

	ftt.Run("Error in execed module", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					def f():
						exec("//exec.star")
					f()
				`,
				"exec.star": `
					def f():
						boom = None()
					f()
				`,
			},
		})
		assert.Loosely(t, normalizeErr(err), should.Equal(`Traceback (most recent call last):
  //main.star: in <toplevel>
  //main.star: in f
Error in exec: exec //exec.star failed: Traceback (most recent call last):
  //exec.star: in <toplevel>
  //exec.star: in f
Error: invalid call of non-function (NoneType)`))
	})

	ftt.Run("Exec cycle", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star":  `exec("//exec1.star")`,
				"exec1.star": `exec("//exec2.star")`,
				"exec2.star": `exec("//exec1.star")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`the module has already been executed, 'exec'-ing same code twice is forbidden`))
	})

	ftt.Run("Trying to exec loaded module", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					load("//mod.star", "z")
					exec("//mod.star")
				`,
				"mod.star": `z = 123`,
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot exec //mod.star: the module has been loaded before and therefore is not executable"))
	})

	ftt.Run("Trying load execed module", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					exec("//mod.star")
					load("//mod.star", "z")
				`,
				"mod.star": `z = 123`,
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot load //mod.star: the module has been exec'ed before and therefore is not loadable"))
	})

	ftt.Run("Trying to exec from loading module", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//mod.star", "z")`,
				"mod.star":  `exec("//zzz.star")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike("exec //zzz.star: forbidden in this context, only exec'ed scripts can exec other scripts"))
	})

	ftt.Run("PreExec/PostExec hooks on success", t, func(t *ftt.Test) {
		var hooks []string
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `exec("@stdlib//exec1.star")`,
			},
			stdlib: map[string]string{
				"exec1.star": `exec("//exec2.star")`,
				"exec2.star": `print("hi")`,
			},
			preExec: func(th *starlark.Thread, module ModuleKey) {
				hooks = append(hooks, fmt.Sprintf("pre %s", module))
			},
			postExec: func(th *starlark.Thread, module ModuleKey) {
				hooks = append(hooks, fmt.Sprintf("post %s", module))
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, hooks, should.Resemble([]string{
			"pre //main.star",
			"pre @stdlib//exec1.star",
			"pre @stdlib//exec2.star",
			"post @stdlib//exec2.star",
			"post @stdlib//exec1.star",
			"post //main.star",
		}))
	})

	ftt.Run("PreExec/PostExec hooks on errors", t, func(t *ftt.Test) {
		var hooks []string
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `exec("@stdlib//exec1.star")`,
			},
			stdlib: map[string]string{
				"exec1.star": `exec("//exec2.star")`,
				"exec2.star": `BOOOM`,
			},
			preExec: func(th *starlark.Thread, module ModuleKey) {
				hooks = append(hooks, fmt.Sprintf("pre %s", module))
			},
			postExec: func(th *starlark.Thread, module ModuleKey) {
				hooks = append(hooks, fmt.Sprintf("post %s", module))
			},
		})
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, hooks, should.Resemble([]string{
			"pre //main.star",
			"pre @stdlib//exec1.star",
			"pre @stdlib//exec2.star",
			"post @stdlib//exec2.star",
			"post @stdlib//exec1.star",
			"post //main.star",
		}))
	})

	ftt.Run("Collects list of visited modules", t, func(t *ftt.Test) {
		var visited []ModuleKey
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					load("//a.star", "sym")
					exec("//c.star")
				`,
				"a.star": `
					load("//b.star", _sym="sym")
					sym = _sym
				`,
				"b.star": `sym = 1`,
				"c.star": `load("//b.star", "sym")`,
			},
			visited: &visited,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, visited, should.Resemble([]ModuleKey{
			{MainPkg, "main.star"},
			{MainPkg, "a.star"},
			{MainPkg, "b.star"},
			{MainPkg, "c.star"},
		}))
	})

	loadSrcBuiltin := starlark.NewBuiltin("load_src", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		src, err := GetThreadInterpreter(th).LoadSource(th, args[0].(starlark.String).GoString())
		return starlark.String(src), err
	})

	ftt.Run("LoadSource works with abs paths", t, func(t *ftt.Test) {
		_, logs, err := runIntr(intrParams{
			predeclared: starlark.StringDict{"load_src": loadSrcBuiltin},
			scripts: map[string]string{
				"main.star": `
					print(load_src("//data1.txt"))
					print(load_src("@stdlib//data2.txt"))
					exec("@stdlib//execed.star")
				`,
				"data1.txt": "blah 1",
			},
			stdlib: map[string]string{
				"execed.star": `print(load_src("//data3.txt"))`,
				"data2.txt":   "blah 2",
				"data3.txt":   "blah 3",
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, logs, should.Resemble([]string{
			"[//main.star:2] blah 1",
			"[//main.star:3] blah 2",
			"[@stdlib//execed.star:1] blah 3",
		}))
	})

	ftt.Run("LoadSource works with rel paths", t, func(t *ftt.Test) {
		_, logs, err := runIntr(intrParams{
			predeclared: starlark.StringDict{"load_src": loadSrcBuiltin},
			scripts: map[string]string{
				"main.star": `
					print(load_src("data1.txt"))
					print(load_src("inner/data2.txt"))
					exec("//inner/execed.star")
					exec("@stdlib//inner/execed.star")
				`,
				"inner/execed.star": `
					print(load_src("../data1.txt"))
					print(load_src("data2.txt"))
				`,
				"data1.txt":       "blah 1",
				"inner/data2.txt": "blah 2",
			},
			stdlib: map[string]string{
				"inner/execed.star": `print(load_src("data3.txt"))`,
				"inner/data3.txt":   "blah 3",
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, logs, should.Resemble([]string{
			"[//main.star:2] blah 1",
			"[//main.star:3] blah 2",
			"[//inner/execed.star:2] blah 1",
			"[//inner/execed.star:3] blah 2",
			"[@stdlib//inner/execed.star:1] blah 3",
		}))
	})

	ftt.Run("LoadSource handles missing files", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			predeclared: starlark.StringDict{"load_src": loadSrcBuiltin},
			scripts: map[string]string{
				"main.star": `load_src("data1.txt")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot load //data1.txt: no such file"))
	})

	ftt.Run("LoadSource handles go modules", t, func(t *ftt.Test) {
		_, _, err := runIntr(intrParams{
			predeclared: starlark.StringDict{"load_src": loadSrcBuiltin},
			scripts: map[string]string{
				"main.star": `load_src("@custom//something.txt")`,
			},
			custom: func(string) (starlark.StringDict, string, error) {
				return starlark.StringDict{}, "", nil
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot load @custom//something.txt: it is a native Go module"))
	})
}
