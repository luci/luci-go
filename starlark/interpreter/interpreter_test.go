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
	"testing"

	"go.starlark.net/starlark"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	// Register proto types in the protobuf lib registry.
	_ "go.chromium.org/luci/starlark/starlarkproto/testprotos"
)

func TestMakeModuleKey(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		k, err := makeModuleKey("//some/mod", nil)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"", "some/mod"})

		k, err = makeModuleKey("//some/mod/../blah", nil)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"", "some/blah"})

		k, err = makeModuleKey("//../../", nil)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"", "../.."})

		k, err = makeModuleKey("@pkg//some/mod", nil)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"pkg", "some/mod"})

		// Picks up default package name from the thread, if given.
		th := starlark.Thread{}
		th.SetLocal(threadPkgKey, "pkg")
		k, err = makeModuleKey("//some/mod", &th)
		So(err, ShouldBeNil)
		So(k, ShouldResemble, moduleKey{"pkg", "some/mod"})
	})

	Convey("Fails", t, func() {
		_, err := makeModuleKey("some/mod", nil)
		So(err, ShouldNotBeNil)

		_, err = makeModuleKey("some//mod", nil)
		So(err, ShouldNotBeNil)

		_, err = makeModuleKey("@//mod", nil)
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
				`,
				"loaded.star": `loaded_sym = "loaded_sym_val"`,
			},
			scripts: map[string]string{
				"main.star": `print(loaded_sym, exported_sym)`,
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
					load("//sub/loaded.star", "loaded_sym")
					load("@stdlib//lib.star", "lib_sym")
					main_sym = True
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

	Convey("Bad module reference", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("../some.star", "some")`,
			},
		})
		So(err, ShouldErrLike, `cannot load ../some.star: a module path should be either '//<path>' or '@<package>//<path>'`)
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
		_, _, err := runIntr(intrParams{
			ctx: context.WithValue(context.Background(), 123, "ctx value"),
			predeclared: starlark.StringDict{
				"call_me": starlark.NewBuiltin("call_me", func(th *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
					fromCtx = Context(th).Value(123).(string)
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
		So(err, ShouldErrLike, "cannot load //mod1.star: cannot load //mod2.star: cannot load //mod1.star: cycle in the module dependency graph")
	})

	Convey("Exec works", t, func() {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					res = exec("//execed.star")
					print(res['a'])
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
		So(err.(*starlark.EvalError).Backtrace(), ShouldEqual, `Traceback (most recent call last):
  //main.star:4: in <toplevel>
  //main.star:3: in f
Error: exec //exec.star failed: Traceback (most recent call last):
  //exec.star:4: in <toplevel>
  //exec.star:3: in f
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
		So(err, ShouldErrLike, "cannot load //mod.star: exec //zzz.star: forbidden in this context, only exec'ed scripts can exec other scripts")
	})
}
