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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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

	ftt.Run("Works", t, func(t *ftt.Test) {
		out, err := runScript(`
def func1():
  return func2()

def func2():
  return stacktrace()

s = func1()

out = str(s)
`)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out, should.Equal(`Traceback (most recent call last):
  main: in <toplevel>
  main: in func1
  main: in func2
  <builtin>: in stacktrace
`))
	})

	ftt.Run("Skips frames", t, func(t *ftt.Test) {
		out, err := runScript(`
def func1():
  return func2()

def func2():
  return stacktrace(skip=2)

out = str(func1())
`)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out, should.Equal(`Traceback (most recent call last):
  main: in <toplevel>
  main: in func1
`))
	})

	ftt.Run("Fails if asked to skip too much", t, func(t *ftt.Test) {
		_, err := runScript(`
def func1():
  return func2()

def func2():
  return stacktrace(skip=5)

out = str(func1())
`)
		assert.Loosely(t, err, should.ErrLike("stacktrace: the stack is not deep enough to skip 5 levels, has only 4 frames"))
	})

	ftt.Run("Fails on negative skip", t, func(t *ftt.Test) {
		_, err := runScript(`stacktrace(-1)`)
		assert.Loosely(t, err, should.ErrLike("stacktrace: bad 'skip' value -1"))
	})

	ftt.Run("Fails on wrong type", t, func(t *ftt.Test) {
		_, err := runScript(`stacktrace('zzz')`)
		assert.Loosely(t, err, should.ErrLike("stacktrace: for parameter skip: got string, want int"))
	})
}

func TestNormalizeStacktrace(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
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
		assert.Loosely(t, NormalizeStacktrace(in), should.Equal(out))
	})
}
