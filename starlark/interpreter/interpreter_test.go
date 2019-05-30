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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	// Register proto types in the protobuf lib registry.
	_ "go.chromium.org/luci/starlark/starlarkproto/testprotos"
)

func TestMakeModuleKey(t *testing.T) {
	t.Parallel()

	th := &starlark.Thread{}
	th.SetLocal(threadModKey, moduleKey{pkg: "cur_pkg", path: "dir/cur.star"})

	Convey("Works", t, func() {
		k, err := makeModuleKey("//some/mod", th)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"cur_pkg", "some/mod"})

		k, err = makeModuleKey("//some/mod/../blah", th)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"cur_pkg", "some/blah"})

		k, err = makeModuleKey("some/mod", th)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"cur_pkg", "dir/some/mod"})

		k, err = makeModuleKey("./mod", th)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"cur_pkg", "dir/mod"})

		k, err = makeModuleKey("../mod", th)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"cur_pkg", "mod"})

		// For absolute paths the thread is optional.
		k, err = makeModuleKey("@pkg//some/mod", nil)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"pkg", "some/mod"})
	})

	Convey("Fails", t, func() {
		_, err := makeModuleKey("@//mod", th)
		So(err, ShouldNotBeNil)

		_, err = makeModuleKey("@mod", th)
		So(err, ShouldNotBeNil)

		// Imports outside of the package root are forbidden.
		_, err = makeModuleKey("//..", th)
		So(err, ShouldNotBeNil)

		_, err = makeModuleKey("../../mod", th)
		So(err, ShouldNotBeNil)

		// If the thread is given, it must have the package name.
		_, err = makeModuleKey("//some/mod", &starlark.Thread{})
		So(err, ShouldNotBeNil)
	})
}

func TestInterpreter(t *testing.T) {
	t.Parallel()

	Convey("Stdlib scripts can load each other", t, func() {
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
		So(err, ShouldBeNil)
		So(keys, ShouldHaveLength, 0) // main.star doesn't export anything itself
		So(logs, ShouldResemble, []string{"[//main.star:1] loaded_sym_val exported_sym_val"})
	})

	Convey("User scripts can load each other and stdlib scripts", t, func() {
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
		So(err, ShouldBeNil)
		So(keys, ShouldResemble, []string{
			"lib_sym",
			"loaded_sym",
			"main_sym",
		})
	})

	Convey("Missing module", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//some.star", "some")`,
			},
		})
		So(err, ShouldErrLike, `cannot load //some.star: no such module`)
	})

	Convey("Missing package", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("@pkg//some.star", "some")`,
			},
		})
		So(err, ShouldErrLike, `cannot load @pkg//some.star: no such package`)
	})

	Convey("Malformed module reference", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("@@", "some")`,
			},
		})
		So(err, ShouldErrLike, `cannot load @@: a module path should be either '//<path>', '<path>' or '@<package>//<path>'`)
	})

	Convey("Double dot module reference", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("../some.star", "some")`,
			},
		})
		So(err, ShouldErrLike, `cannot load ../some.star: outside the package root`)
	})

	Convey("Predeclared are exposed to stdlib and user scripts", t, func() {
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
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[@stdlib//builtins.star:1] 123",
			"[//main.star:1] 123",
		})
	})

	Convey("Predeclared can access the context", t, func() {
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
		So(err, ShouldBeNil)
		So(fromCtx, ShouldEqual, "ctx value")
	})

	Convey("Modules are loaded only once", t, func() {
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
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[//mod.star:2] Loading", // only once
			"[//main.star:5] 1 2",
		})
	})

	Convey("Module cycles are caught", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//mod1.star", "a")`,
				"mod1.star": `load("//mod2.star", "a")`,
				"mod2.star": `load("//mod1.star", "a")`,
			},
		})
		So(normalizeErr(err), ShouldEqual, `Traceback (most recent call last):
  //main.star: in <toplevel>
Error: cannot load //mod1.star: Traceback (most recent call last):
  //mod1.star: in <toplevel>
Error: cannot load //mod2.star: Traceback (most recent call last):
  //mod2.star: in <toplevel>
Error: cannot load //mod1.star: cycle in the module dependency graph`)
	})

	Convey("Error in loaded module", t, func() {
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
		So(normalizeErr(err), ShouldEqual, `Traceback (most recent call last):
  //main.star: in <toplevel>
Error: cannot load //mod.star: Traceback (most recent call last):
  //mod.star: in <toplevel>
  //mod.star: in f
Error: invalid call of non-function (NoneType)`)
	})

	Convey("Exec works", t, func() {
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
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[//execed.star:2] hi",
			"[//main.star:3] 123",
		})
	})

	Convey("Exec using relative path", t, func() {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star":  `exec("//sub/1.star")`,
				"sub/1.star": `exec("./2.star")`,
				"sub/2.star": `print('hi')`,
			},
		})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[//sub/2.star:1] hi",
		})
	})

	Convey("Exec into another package", t, func() {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `exec("@stdlib//exec1.star")`,
			},
			stdlib: map[string]string{
				"exec1.star": `exec("//exec2.star")`,
				"exec2.star": `print("hi")`,
			},
		})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[@stdlib//exec2.star:1] hi",
		})
	})

	Convey("Error in execed module", t, func() {
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
		So(normalizeErr(err), ShouldEqual, `Traceback (most recent call last):
  //main.star: in <toplevel>
  //main.star: in f
  <builtin>: in exec
Error: exec //exec.star failed: Traceback (most recent call last):
  //exec.star: in <toplevel>
  //exec.star: in f
Error: invalid call of non-function (NoneType)`)
	})

	Convey("Exec cycle", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star":  `exec("//exec1.star")`,
				"exec1.star": `exec("//exec2.star")`,
				"exec2.star": `exec("//exec1.star")`,
			},
		})
		So(err, ShouldErrLike, `the module has already been executed, 'exec'-ing same code twice is forbidden`)
	})

	Convey("Trying to exec loaded module", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					load("//mod.star", "z")
					exec("//mod.star")
				`,
				"mod.star": `z = 123`,
			},
		})
		So(err, ShouldErrLike, "cannot exec //mod.star: the module has been loaded before and therefore is not executable")
	})

	Convey("Trying load execed module", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					exec("//mod.star")
					load("//mod.star", "z")
				`,
				"mod.star": `z = 123`,
			},
		})
		So(err, ShouldErrLike, "cannot load //mod.star: the module has been exec'ed before and therefore is not loadable")
	})

	Convey("Trying to exec from loading module", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//mod.star", "z")`,
				"mod.star":  `exec("//zzz.star")`,
			},
		})
		So(err, ShouldErrLike, "exec //zzz.star: forbidden in this context, only exec'ed scripts can exec other scripts")
	})

	Convey("PreExec/PostExec hooks on success", t, func() {
		var hooks []string
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `exec("@stdlib//exec1.star")`,
			},
			stdlib: map[string]string{
				"exec1.star": `exec("//exec2.star")`,
				"exec2.star": `print("hi")`,
			},
			preExec: func(th *starlark.Thread, pkg, path string) {
				hooks = append(hooks, fmt.Sprintf("pre %s/%s", pkg, path))
			},
			postExec: func(th *starlark.Thread, pkg, path string) {
				hooks = append(hooks, fmt.Sprintf("post %s/%s", pkg, path))
			},
		})
		So(err, ShouldBeNil)
		So(hooks, ShouldResemble, []string{
			"pre __main__/main.star",
			"pre stdlib/exec1.star",
			"pre stdlib/exec2.star",
			"post stdlib/exec2.star",
			"post stdlib/exec1.star",
			"post __main__/main.star",
		})
	})

	Convey("PreExec/PostExec hooks on errors", t, func() {
		var hooks []string
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `exec("@stdlib//exec1.star")`,
			},
			stdlib: map[string]string{
				"exec1.star": `exec("//exec2.star")`,
				"exec2.star": `BOOOM`,
			},
			preExec: func(th *starlark.Thread, pkg, path string) {
				hooks = append(hooks, fmt.Sprintf("pre %s/%s", pkg, path))
			},
			postExec: func(th *starlark.Thread, pkg, path string) {
				hooks = append(hooks, fmt.Sprintf("post %s/%s", pkg, path))
			},
		})
		So(err, ShouldNotBeNil)
		So(hooks, ShouldResemble, []string{
			"pre __main__/main.star",
			"pre stdlib/exec1.star",
			"pre stdlib/exec2.star",
			"post stdlib/exec2.star",
			"post stdlib/exec1.star",
			"post __main__/main.star",
		})
	})

	loadSrcBuiltin := starlark.NewBuiltin("load_src", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		src, err := GetThreadInterpreter(th).LoadSource(th, args[0].(starlark.String).GoString())
		return starlark.String(src), err
	})

	Convey("LoadSource works with abs paths", t, func() {
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
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[//main.star:2] blah 1",
			"[//main.star:3] blah 2",
			"[@stdlib//execed.star:1] blah 3",
		})
	})

	Convey("LoadSource works with rel paths", t, func() {
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
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[//main.star:2] blah 1",
			"[//main.star:3] blah 2",
			"[//inner/execed.star:2] blah 1",
			"[//inner/execed.star:3] blah 2",
			"[@stdlib//inner/execed.star:1] blah 3",
		})
	})

	Convey("LoadSource handles missing files", t, func() {
		_, _, err := runIntr(intrParams{
			predeclared: starlark.StringDict{"load_src": loadSrcBuiltin},
			scripts: map[string]string{
				"main.star": `load_src("data1.txt")`,
			},
		})
		So(err, ShouldErrLike, "cannot load //data1.txt: no such file")
	})

	Convey("LoadSource handles go modules", t, func() {
		_, _, err := runIntr(intrParams{
			predeclared: starlark.StringDict{"load_src": loadSrcBuiltin},
			scripts: map[string]string{
				"main.star": `load_src("@custom//something.txt")`,
			},
			custom: func(string) (starlark.StringDict, string, error) {
				return starlark.StringDict{}, "", nil
			},
		})
		So(err, ShouldErrLike, "cannot load @custom//something.txt: it is a native Go module")
	})
}
