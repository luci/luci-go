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

	"github.com/google/skylark"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	// Register proto types in the protobuf lib registry.
	_ "go.chromium.org/luci/skylark/skylarkproto/testprotos"
)

func TestInterpreter(t *testing.T) {
	t.Parallel()

	Convey("Stdlib scripts can load each other", t, func() {
		keys, logs, err := runIntr(intrParams{
			stdlib: map[string]string{
				"init.sky": `
					load("builtin:loaded.sky", "loaded_sym")
					exported_sym = "exported_sym_val"
				`,
				"loaded.sky": `loaded_sym = "loaded_sym_val"`,
			},
			scripts: map[string]string{
				"main.sky": `print(loaded_sym, exported_sym)`,
			},
		})
		So(err, ShouldBeNil)
		So(keys, ShouldHaveLength, 0) // main.sky doesn't export anything itself
		So(logs, ShouldResemble, []string{"[main.sky:1] loaded_sym_val exported_sym_val"})
	})

	Convey("User scripts can load each other and stdlib scripts", t, func() {
		keys, _, err := runIntr(intrParams{
			stdlib: map[string]string{
				"lib.sky": `lib_sym = True`,
			},
			scripts: map[string]string{
				"main.sky": `
					# Absolute paths work.
					load("//sub/loaded.sky", "loaded_sym", "another_loaded_sym")
					# Loading stdlib modules work.
					load("builtin:lib.sky", "lib_sym")
					main_sym = True
				`,
				"sub/loaded.sky": `
					# Relative paths work.
					load("../another_loaded.sky", "another_loaded_sym")
					loaded_sym = True
				`,
				"another_loaded.sky": `another_loaded_sym = True`,
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
				"init.sky": `load("//some.sky", "some")`,
			},
		})
		So(err, ShouldErrLike, `builtin module "builtin:init.sky" is attempting to load non-builtin`)
	})

	Convey("Missing scripts", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.sky": `load("//some.sky", "some")`,
			},
		})
		So(err, ShouldErrLike, `failed to read`)
	})

	Convey("Predeclared are exposed to stdlib and user scripts", t, func() {
		_, logs, err := runIntr(intrParams{
			predeclared: skylark.StringDict{
				"imported_sym": skylark.MakeInt(123),
			},
			stdlib: map[string]string{
				"init.sky": `print(imported_sym)`,
			},
			scripts: map[string]string{
				"main.sky": `print(imported_sym)`,
			},
		})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[builtin:init.sky:1] 123",
			"[main.sky:1] 123",
		})
	})

	Convey("Can load protos and have 'proto' module available", t, func() {
		_, logs, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.sky": `
					load("builtin:go.chromium.org/luci/skylark/skylarkproto/testprotos/test.proto", "testprotos")
					print(proto.to_pbtext(testprotos.SimpleFields(i64=123)))
				`,
			},
		})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{"[main.sky:3] i64: 123\n"})
	})

	Convey("Modules are loaded only once", t, func() {
		_, logs, err := runIntr(intrParams{
			stdlib: map[string]string{
				"init.sky": `
					load("builtin:mod.sky", "a")
					load("builtin:mod.sky", "b")

					print(a, b)
				`,
				"mod.sky": `
					print("Loading")

					a = 1
					b = 2
				`,
			},
			scripts: map[string]string{
				"main.sky": `
					load("mod.sky", "a")
					load("mod.sky", "b")

					print(a, b)
				`,
				"mod.sky": `
					print("Loading")

					a = 1
					b = 2
				`,
			},
		})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[builtin:mod.sky:2] Loading",
			"[builtin:init.sky:5] 1 2",
			"[mod.sky:2] Loading",
			"[main.sky:5] 1 2",
		})
	})

	Convey("Load cycles are caught", t, func() {
		_, _, err := runIntr(intrParams{
			scripts: map[string]string{
				"main.sky": `load("mod.sky", "a")`,
				"mod.sky":  `load("main.sky", "a")`,
			},
		})
		So(err, ShouldErrLike, "cannot load mod.sky: cannot load main.sky: cannot load mod.sky: cycle in load graph")
	})
}
