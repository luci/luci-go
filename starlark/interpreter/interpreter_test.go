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
	"testing"

	"go.starlark.net/starlark"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	// Register proto types in the protobuf lib registry.
	_ "go.chromium.org/luci/starlark/starlarkproto/testprotos"
)

func TestInterpreter(t *testing.T) {
	t.Parallel()

	Convey("Stdlib scripts can load each other", t, func() {
		keys, logs, err := runIntr(intrParams{
			stdlib: map[string]string{
				"init.star": `
					load("builtin:loaded.star", "loaded_sym")
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
		So(logs, ShouldResemble, []string{"[main.star:1] loaded_sym_val exported_sym_val"})
	})

	Convey("User scripts can load each other and stdlib scripts", t, func() {
		keys, _, err := runIntr(intrParams{
			stdlib: map[string]string{
				"lib.star": `lib_sym = True`,
			},
			scripts: map[string]string{
				"main.star": `
					# Absolute paths work.
					load("//sub/loaded.star", "loaded_sym", "another_loaded_sym")
					# Loading stdlib modules work.
					load("builtin:lib.star", "lib_sym")
					main_sym = True
				`,
				"sub/loaded.star": `
					# Relative paths work.
					load("../another_loaded.star", "another_loaded_sym")
					loaded_sym = True
				`,
				"another_loaded.star": `another_loaded_sym = True`,
			},
		})
		So(err, ShouldBeNil)
		So(keys, ShouldResemble, []string{
			"another_loaded_sym",
			"lib_sym",
			"loaded_sym",
			"main_sym",
		})
	})

	Convey("Stdlib scripts can NOT load non-stdlib scripts", t, func() {
		_, _, err := runIntr(intrParams{
			stdlib: map[string]string{
				"init.star": `load("//some.star", "some")`,
			},
		})
		So(err, ShouldErrLike, `builtin module "builtin:init.star" is attempting to load non-builtin`)
	})

	Convey("Missing scripts", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("//some.star", "some")`,
			},
		})
		So(err, ShouldErrLike, `failed to read`)
	})

	Convey("Predeclared are exposed to stdlib and user scripts", t, func() {
		_, logs, err := runIntr(intrParams{
			predeclared: starlark.StringDict{
				"imported_sym": starlark.MakeInt(123),
			},
			stdlib: map[string]string{
				"init.star": `print(imported_sym)`,
			},
			scripts: map[string]string{
				"main.star": `print(imported_sym)`,
			},
		})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[builtin:init.star:1] 123",
			"[main.star:1] 123",
		})
	})

	Convey("Can load protos and have 'proto' module available", t, func() {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `
					load("builtin:go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto", "testprotos")
					print(proto.to_pbtext(testprotos.SimpleFields(i64=123)))
				`,
			},
		})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{"[main.star:3] i64: 123\n"})
	})

	Convey("Modules are loaded only once", t, func() {
		_, logs, err := runIntr(intrParams{
			stdlib: map[string]string{
				"init.star": `
					load("builtin:mod.star", "a")
					load("builtin:mod.star", "b")

					print(a, b)
				`,
				"mod.star": `
					print("Loading")

					a = 1
					b = 2
				`,
			},
			scripts: map[string]string{
				"main.star": `
					load("mod.star", "a")
					load("mod.star", "b")

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
			"[builtin:mod.star:2] Loading",
			"[builtin:init.star:5] 1 2",
			"[mod.star:2] Loading",
			"[main.star:5] 1 2",
		})
	})

	Convey("Load cycles are caught", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.star": `load("mod.star", "a")`,
				"mod.star":  `load("main.star", "a")`,
			},
		})
		So(err, ShouldErrLike, "cannot load mod.star: cannot load main.star: cannot load mod.star: cycle in load graph")
	})
}
