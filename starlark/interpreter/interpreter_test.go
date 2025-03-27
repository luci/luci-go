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
	"errors"
	"fmt"
	"testing"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMakeModuleKey(t *testing.T) {
	t.Parallel()

	th := &starlark.Thread{}
	th.SetLocal(threadModKey, ModuleKey{"cur_pkg", "dir/cur.star"})

	t.Run("Works", func(t *testing.T) {
		k, err := MakeModuleKey(th, "//some/mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Match(ModuleKey{"cur_pkg", "some/mod"}))

		k, err = MakeModuleKey(th, "//some/mod/../blah")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Match(ModuleKey{"cur_pkg", "some/blah"}))

		k, err = MakeModuleKey(th, "some/mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Match(ModuleKey{"cur_pkg", "dir/some/mod"}))

		k, err = MakeModuleKey(th, "./mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Match(ModuleKey{"cur_pkg", "dir/mod"}))

		k, err = MakeModuleKey(th, "../mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Match(ModuleKey{"cur_pkg", "mod"}))

		// For absolute paths the thread is optional.
		k, err = MakeModuleKey(nil, "@pkg//some/mod")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Match(ModuleKey{"pkg", "some/mod"}))
	})

	t.Run("Fails", func(t *testing.T) {
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

	t.Run("Stdlib scripts can load each other", func(t *testing.T) {
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
		assert.Loosely(t, logs, should.Match([]string{"[//main.star:1] loaded_sym_val exported_sym_val"}))
	})

	t.Run("User scripts can load each other and stdlib scripts", func(t *testing.T) {
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
		assert.Loosely(t, keys, should.Match([]string{
			"lib_sym",
			"loaded_sym",
			"main_sym",
		}))
	})

	t.Run("Missing module", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//some.star", "some")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`cannot load //some.star: no such module`))
	})

	t.Run("Missing package", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("@pkg//some.star", "some")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`cannot load @pkg//some.star: no such package`))
	})

	t.Run("Malformed module reference", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("@@", "some")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`cannot load @@: a module path should be either '//<path>', '<path>' or '@<package>//<path>'`))
	})

	t.Run("Double dot module reference", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("../some.star", "some")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`cannot load ../some.star: outside the package root`))
	})

	t.Run("Predeclared are exposed to stdlib and user scripts", func(t *testing.T) {
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
		assert.Loosely(t, logs, should.Match([]string{
			"[@stdlib//builtins.star:1] 123",
			"[//main.star:1] 123",
		}))
	})

	t.Run("Predeclared can access the context", func(t *testing.T) {
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

	t.Run("Modules are loaded only once", func(t *testing.T) {
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
		assert.Loosely(t, logs, should.Match([]string{
			"[//mod.star:2] Loading", // only once
			"[//main.star:5] 1 2",
		}))
	})

	t.Run("Module cycles are caught", func(t *testing.T) {
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

	t.Run("Error in loaded module", func(t *testing.T) {
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

	t.Run("Exec works", func(t *testing.T) {
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
		assert.Loosely(t, logs, should.Match([]string{
			"[//execed.star:2] hi",
			"[//main.star:3] 123",
		}))
	})

	t.Run("Exec using relative path", func(t *testing.T) {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star":  `exec("//sub/1.star")`,
				"sub/1.star": `exec("./2.star")`,
				"sub/2.star": `print('hi')`,
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, logs, should.Match([]string{
			"[//sub/2.star:1] hi",
		}))
	})

	t.Run("Exec into another package", func(t *testing.T) {
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
		assert.Loosely(t, logs, should.Match([]string{
			"[@stdlib//exec2.star:1] hi",
		}))
	})

	t.Run("Error in execed module", func(t *testing.T) {
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

	t.Run("Exec cycle", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star":  `exec("//exec1.star")`,
				"exec1.star": `exec("//exec2.star")`,
				"exec2.star": `exec("//exec1.star")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike(`the module has already been executed, 'exec'-ing same code twice is forbidden`))
	})

	t.Run("Trying to exec loaded module", func(t *testing.T) {
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

	t.Run("Trying load execed module", func(t *testing.T) {
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

	t.Run("Trying to exec from loading module", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//mod.star", "z")`,
				"mod.star":  `exec("//zzz.star")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike("exec //zzz.star: forbidden in this context, only exec'ed scripts can exec other scripts"))
	})

	t.Run("PreExec/PostExec hooks on success", func(t *testing.T) {
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
				hooks = append(hooks, fmt.Sprintf("pre %s", module.Friendly(th)))
			},
			postExec: func(th *starlark.Thread, module ModuleKey) {
				hooks = append(hooks, fmt.Sprintf("post %s", module.Friendly(th)))
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, hooks, should.Match([]string{
			"pre //main.star",
			"pre @stdlib//exec1.star",
			"pre @stdlib//exec2.star",
			"post @stdlib//exec2.star",
			"post @stdlib//exec1.star",
			"post //main.star",
		}))
	})

	t.Run("PreExec/PostExec hooks on errors", func(t *testing.T) {
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
				hooks = append(hooks, fmt.Sprintf("pre %s", module.Friendly(th)))
			},
			postExec: func(th *starlark.Thread, module ModuleKey) {
				hooks = append(hooks, fmt.Sprintf("post %s", module.Friendly(th)))
			},
		})
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, hooks, should.Match([]string{
			"pre //main.star",
			"pre @stdlib//exec1.star",
			"pre @stdlib//exec2.star",
			"post @stdlib//exec2.star",
			"post @stdlib//exec1.star",
			"post //main.star",
		}))
	})

	t.Run("Collects list of visited modules", func(t *testing.T) {
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
		assert.Loosely(t, visited, should.Match([]ModuleKey{
			{mainPkg, "main.star"},
			{mainPkg, "a.star"},
			{mainPkg, "b.star"},
			{mainPkg, "c.star"},
		}))
	})

	loadSrcBuiltin := starlark.NewBuiltin("load_src", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		src, err := GetThreadInterpreter(th).LoadSource(th, args[0].(starlark.String).GoString())
		return starlark.String(src), err
	})

	t.Run("LoadSource works with abs paths", func(t *testing.T) {
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
		assert.Loosely(t, logs, should.Match([]string{
			"[//main.star:2] blah 1",
			"[//main.star:3] blah 2",
			"[@stdlib//execed.star:1] blah 3",
		}))
	})

	t.Run("LoadSource works with rel paths", func(t *testing.T) {
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
		assert.Loosely(t, logs, should.Match([]string{
			"[//main.star:2] blah 1",
			"[//main.star:3] blah 2",
			"[//inner/execed.star:2] blah 1",
			"[//inner/execed.star:3] blah 2",
			"[@stdlib//inner/execed.star:1] blah 3",
		}))
	})

	t.Run("LoadSource handles missing files", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			predeclared: starlark.StringDict{"load_src": loadSrcBuiltin},
			scripts: map[string]string{
				"main.star": `load_src("data1.txt")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot load //data1.txt: no such file"))
	})

	t.Run("LoadSource handles go modules", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			predeclared: starlark.StringDict{"load_src": loadSrcBuiltin},
			scripts: map[string]string{
				"main.star": `load_src("@custom//something.txt")`,
			},
			custom: func(context.Context, string) (starlark.StringDict, string, error) {
				return starlark.StringDict{}, "", nil
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot load @custom//something.txt: it is a native Go module"))
	})

	t.Run("ForbidLoad works", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			forbidLoad: "boooo",
			scripts: map[string]string{
				"main.star": `load("//mod.star", "z")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot load //mod.star: boooo"))
	})

	t.Run("ForbidExec works", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			forbidExec: "boooo",
			scripts: map[string]string{
				"main.star": `exec("//mod.star")`,
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot exec //mod.star: boooo"))
	})

	t.Run("CheckVisible: pass", func(t *testing.T) {
		var checks []string
		_, _, err := runIntr(intrParams{
			stdlib: map[string]string{
				"lib.star": `loaded_sym = True`,
			},
			scripts: map[string]string{
				"main.star": `
					load("loaded.star", _loaded_sym1="loaded_sym")
					exec("execed.star")
					load("@stdlib//lib.star", _loaded_sym2="loaded_sym")
				`,
				"loaded.star": `loaded_sym = True`,
				"execed.star": `
					load("loaded.star", _loaded_sym1="loaded_sym")
					load("@stdlib//lib.star", _loaded_sym2="loaded_sym")
				`,
			},
			checkVisible: func(_ *starlark.Thread, loader, loaded ModuleKey) error {
				checks = append(checks, fmt.Sprintf("%s -> %s", loader, loaded))
				return nil
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, checks, should.Match([]string{
			"@test_main//main.star -> @test_main//loaded.star",
			"@test_main//main.star -> @test_main//execed.star",
			"@test_main//execed.star -> @test_main//loaded.star",
			"@test_main//execed.star -> @stdlib//lib.star",
			"@test_main//main.star -> @stdlib//lib.star",
		}))
	})

	t.Run("CheckVisible: deny load", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					load("loaded.star", _loaded_sym1="loaded_sym")
				`,
				"loaded.star": `will never be used`,
			},
			checkVisible: func(_ *starlark.Thread, loader, loaded ModuleKey) error {
				return errors.New("boo")
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot load loaded.star: boo"))
	})

	t.Run("CheckVisible: deny exec", func(t *testing.T) {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					exec("execed.star")
				`,
				"execed.star": `will never be used`,
			},
			checkVisible: func(_ *starlark.Thread, loader, loaded ModuleKey) error {
				return errors.New("boo")
			},
		})
		assert.Loosely(t, err, should.ErrLike("cannot exec execed.star: boo"))
	})
}
