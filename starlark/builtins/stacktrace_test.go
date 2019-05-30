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

package builtins

import (
	"fmt"
	"testing"

	"go.starlark.net/starlark"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStacktrace(t *testing.T) {
	t.Parallel()

	runScript := func(code string) (string, error) {
		out, err := starlark.ExecFile(&starlark.Thread{}, "main", code, starlark.StringDict{
			"stacktrace": Stacktrace,
		})
		if err != nil {
			return "", err
		}
		if s, ok := out["out"].(starlark.String); ok {
			return NormalizeStacktrace(s.GoString()), nil
		}
		return "", fmt.Errorf("not a string: %s", out["out"])
	}

	Convey("Works", t, func() {
		out, err := runScript(`
def func1():
  return func2()

def func2():
  return stacktrace()

s = func1()

out = str(s)
`)
		So(err, ShouldBeNil)
		So(out, ShouldEqual, `Traceback (most recent call last):
  main: in <toplevel>
  main: in func1
  main: in func2
  <builtin>: in stacktrace
`)
	})

	Convey("Skips frames", t, func() {
		out, err := runScript(`
def func1():
  return func2()

def func2():
  return stacktrace(skip=2)

out = str(func1())
`)
		So(err, ShouldBeNil)
		So(out, ShouldEqual, `Traceback (most recent call last):
  main: in <toplevel>
  main: in func1
`)
	})

	Convey("Fails if asked to skip too much", t, func() {
		_, err := runScript(`
def func1():
  return func2()

def func2():
  return stacktrace(skip=5)

out = str(func1())
`)
		So(err, ShouldErrLike, "stacktrace: the stack is not deep enough to skip 5 levels, has only 4 frames")
	})

	Convey("Fails on negative skip", t, func() {
		_, err := runScript(`stacktrace(-1)`)
		So(err, ShouldErrLike, "stacktrace: bad 'skip' value -1")
	})

	Convey("Fails on wrong type", t, func() {
		_, err := runScript(`stacktrace('zzz')`)
		So(err, ShouldErrLike, "stacktrace: for parameter skip: got string, want int")
	})
}

func TestNormalizeStacktrace(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		in := `Traceback (most recent call last):
  main:8:1: in <toplevel>
  main:3:2: in func1
  main:6:3: in func2
  <builtin>: in stacktrace

  skipped line
`
		out := `Traceback (most recent call last):
  main: in <toplevel>
  main: in func1
  main: in func2
  <builtin>: in stacktrace

  skipped line
`
		So(NormalizeStacktrace(in), ShouldEqual, out)
	})
}
