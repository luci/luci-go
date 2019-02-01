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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	// Register proto types in the protobuf lib registry.
	_ "go.chromium.org/luci/starlark/starlarkproto/testprotos"
)

// runs a script in an environment where 'custom' package uses the given loader.
func runScriptWithLoader(body string, l Loader) (logs []string, err error) {
	_, logs, err = runIntr(intrParams{
		stdlib: map[string]string{"builtins.star": body},
		custom: l,
	})
	return logs, err
}

func TestLoaders(t *testing.T) {
	t.Parallel()

	Convey("FileSystemLoader", t, func() {
		tmp, err := ioutil.TempDir("", "starlark")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tmp)

		loader := FileSystemLoader(tmp)

		put := func(path, body string) {
			path = filepath.Join(tmp, filepath.FromSlash(path))
			So(os.MkdirAll(filepath.Dir(path), 0700), ShouldBeNil)
			So(ioutil.WriteFile(path, []byte(body), 0600), ShouldBeNil)
		}

		Convey("Works", func() {
			put("1.star", `load("//a/b/c/2.star", "sym")`)
			put("a/b/c/2.star", "print('Hi')\nsym = 1")

			logs, err := runScriptWithLoader(`load("@custom//1.star", "sym")`, loader)
			So(err, ShouldBeNil)
			So(logs, ShouldResemble, []string{
				"[@custom//a/b/c/2.star:1] Hi",
			})
		})

		Convey("Missing module", func() {
			put("1.star", `load("//a/b/c/2.star", "sym")`)

			_, err := runScriptWithLoader(`load("@custom//1.star", "sym")`, loader)
			So(err, ShouldErrLike, "cannot load //a/b/c/2.star: no such module")
		})

		Convey("Outside the root", func() {
			_, err := runScriptWithLoader(`load("@custom//../1.star", "sym")`, loader)
			So(err, ShouldErrLike, "cannot load @custom//../1.star: outside the package root")
		})
	})

	Convey("MemoryLoader", t, func() {
		Convey("Works", func() {
			loader := MemoryLoader(map[string]string{
				"1.star":       `load("//a/b/c/2.star", "sym")`,
				"a/b/c/2.star": "print('Hi')\nsym = 1",
			})

			logs, err := runScriptWithLoader(`load("@custom//1.star", "sym")`, loader)
			So(err, ShouldBeNil)
			So(logs, ShouldResemble, []string{
				"[@custom//a/b/c/2.star:1] Hi",
			})
		})

		Convey("Missing module", func() {
			loader := MemoryLoader(map[string]string{
				"1.star": `load("//a/b/c/2.star", "sym")`,
			})

			_, err := runScriptWithLoader(`load("@custom//1.star", "sym")`, loader)
			So(err, ShouldErrLike, "cannot load //a/b/c/2.star: no such module")
		})
	})

	Convey("ProtoLoader", t, func() {
		Convey("Works", func() {
			l := ProtoLoader(map[string]string{
				"test.proto": "go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto",
			})
			logs, err := runScriptWithLoader(`
				load("@custom//test.proto", "testprotos")
				print(testprotos.SimpleFields(i64=123))
			`, l)
			So(err, ShouldBeNil)
			So(logs, ShouldResemble, []string{
				"[@stdlib//builtins.star:3] i64:123 ",
			})
		})

		Convey("Not allowed proto", func() {
			l := ProtoLoader(nil) // empty whitelist of allowed protos
			_, err := runScriptWithLoader(`load("@custom//nope.proto", "test")`, l)
			So(err, ShouldErrLike, "cannot load @custom//nope.proto: no such module")
		})

		Convey("Unknown proto", func() {
			l := ProtoLoader(map[string]string{
				"unknown.proto": "not_in_the_registry.proto",
			})
			_, err := runScriptWithLoader(`load("@custom//unknown.proto", "test")`, l)
			So(err, ShouldErrLike, "cannot load @custom//unknown.proto: no such proto file registered")
		})
	})
}
