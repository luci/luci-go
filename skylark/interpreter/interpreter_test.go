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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
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
		_, _, err := run(Interpreter{Root: "testdata/1"})
		So(err, ShouldBeNil)
	})

	Convey("User scripts can load each other and stdlib scripts", t, func() {
		keys, _, err := run(Interpreter{Root: "testdata/2"})
		So(err, ShouldBeNil)
		So(keys, ShouldResemble, []string{
			"another_loaded_sym",
			"lib_sym",
			"loaded_sym",
			"main_sym",
		})
	})

	Convey("Stdlib scripts can NOT load non-stdlib scripts", t, func() {
		_, _, err := run(Interpreter{Root: "testdata/3"})
		So(err, ShouldErrLike, `builtin module "builtin:init.sky" is attempting to load non-builtin`)
	})

	Convey("Missing scripts", t, func() {
		_, _, err := run(Interpreter{Root: "testdata/missing"})
		So(err, ShouldErrLike, `failed to read`)
	})

	Convey("Builtins are exposed to stdlib and user scripts", t, func() {
		_, logs, err := run(Interpreter{
			Root: "testdata/4",
			Builtins: skylark.StringDict{
				"imported_builtin": skylark.MakeInt(123),
			},
		})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[builtin:init.sky:1] 123",
			"[main.sky:1] 123",
		})
	})

	Convey("Can load protos and have 'proto' module available", t, func() {
		_, logs, err := run(Interpreter{Root: "testdata/5"})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{"[main.sky:2] i64: 123\n"})
	})

	Convey("Modules are loaded only once", t, func() {
		_, logs, err := run(Interpreter{Root: "testdata/6"})
		So(err, ShouldBeNil)
		So(logs, ShouldResemble, []string{
			"[builtin:mod.sky:1] Loading",
			"[builtin:init.sky:4] 1, 2",
			"[mod.sky:1] Loading",
			"[main.sky:4] 1, 2",
		})
	})

	Convey("Load cycles are caught", t, func() {
		_, _, err := run(Interpreter{Root: "testdata/7"})
		So(err, ShouldErrLike, "cannot load mod.sky: cannot load main.sky: cannot load mod.sky: cycle in load graph")
	})
}

func run(intr Interpreter) (keys []string, logs []string, err error) {
	intr.Stdlib = func(path string) (string, error) {
		body, err := ioutil.ReadFile(filepath.Join(intr.Root, "stdlib", path))
		if os.IsNotExist(err) {
			return "", ErrNoStdlibModule
		}
		return string(body), nil
	}

	intr.Logger = func(file string, line int, message string) {
		logs = append(logs, fmt.Sprintf("[%s:%d] %s", file, line, message))
	}

	if err = intr.Init(); err != nil {
		return
	}

	dict, err := intr.ExecFile(filepath.Join(intr.Root, "/main.sky"))
	if err != nil {
		return
	}

	keys = make([]string, 0, len(dict))
	for k := range dict {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return
}
