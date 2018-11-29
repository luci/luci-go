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
	"testing"

	"go.starlark.net/starlark"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStacktrace(t *testing.T) {
	t.Parallel()

	runScript := func(code string) (starlark.Value, error) {
		out, err := starlark.ExecFile(&starlark.Thread{}, "main", code, starlark.StringDict{
			"stacktrace": Stacktrace,
		})
		if err != nil {
			return nil, err
		}
		return out["out"], nil
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
		So(out.(starlark.String).GoString(), ShouldEqual, `Traceback (most recent call last):
  main:8: in <toplevel>
  main:3: in func1
  main:6: in func2
`)
	})

	Convey("Skips frames", t, func() {
		out, err := runScript(`
def func1():
  return func2()

def func2():
  return stacktrace(skip=1)

out = str(func1())
`)
		So(err, ShouldBeNil)
		So(out.(starlark.String).GoString(), ShouldEqual, `Traceback (most recent call last):
  main:8: in <toplevel>
  main:3: in func1
`)
	})

	Convey("Fails if asked to skip too much", t, func() {
		_, err := runScript(`
def func1():
  return func2()

def func2():
  return stacktrace(skip=4)

out = str(func1())
`)
		So(err, ShouldErrLike, "stacktrace: the stack is not deep enough to skip 4 levels, has only 3 frames")
	})

	Convey("Fails on negative skip", t, func() {
		_, err := runScript(`stacktrace(-1)`)
		So(err, ShouldErrLike, "stacktrace: bad 'skip' value -1")
	})

	Convey("Fails on wrong type", t, func() {
		_, err := runScript(`stacktrace('zzz')`)
		So(err, ShouldErrLike, "stacktrace: for parameter 1: got string, want int")
	})
}
