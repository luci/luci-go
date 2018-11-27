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

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	resolve.AllowFloat = true
	resolve.AllowSet = true
}

func TestToJSON(t *testing.T) {
	t.Parallel()

	runScript := func(code string) (starlark.StringDict, error) {
		return starlark.ExecFile(&starlark.Thread{}, "main", code, starlark.StringDict{
			"to_json": ToJSON,
		})
	}

	run := func(val string, expected string) {
		out, err := runScript(fmt.Sprintf("result = to_json(%s)", val))
		So(err, ShouldBeNil)
		So(out["result"].(starlark.String).GoString(), ShouldEqual, expected)
	}

	mustFail := func(val string, expectErr string) {
		_, err := runScript(fmt.Sprintf("result = to_json(%s)", val))
		So(err, ShouldErrLike, expectErr)
	}

	Convey("Happy cases", t, func() {
		Convey("Scalar types", func() {
			run(`None`, `null`)
			run(`True`, `true`)
			run(`123`, `123`)
			run(`123.5`, `123.5`)
			run(`"hi"`, `"hi"`)
		})

		Convey("Dict", func() {
			run(`{}`, `{}`)
			run(`{"a": 1, "b": 2}`, `{"a":1,"b":2}`)
		})

		Convey("Iterables", func() {
			run(`[]`, `[]`)
			run(`()`, `[]`)
			run(`set()`, `[]`)
			run(`[1, 2, 3]`, `[1,2,3]`)
			run(`(1, 2, 3)`, `[1,2,3]`)
			run(`set([1, 2, 3])`, `[1,2,3]`)
		})

		Convey("Everything at once", func() {
			run(`{"a": None, "b": ["c", "d", ["e"]]}`, `{"a":null,"b":["c","d",["e"]]}`)
		})
	})

	Convey("Unhappy cases", t, func() {
		mustFail(`list`, `to_json: unsupported type builtin_function_or_method`)
		mustFail(`18446744073709551616`, `to_json: can't convert "18446744073709551616" to int64`)
		mustFail(`{1: 2}`, `to_json: dict keys should be strings, got int`)

		// Errors propagate when serializing containers.
		mustFail(`{"a": list}`, `to_json: unsupported type builtin_function_or_method`)
		mustFail(`[list]`, `to_json: unsupported type builtin_function_or_method`)
		mustFail(`(list, list)`, `to_json: unsupported type builtin_function_or_method`)
		mustFail(`set([list])`, `to_json: unsupported type builtin_function_or_method`)
	})

	Convey("Detects recursive structures", t, func() {
		const msg = "to_json: detected recursion in the data structure"

		// With dict.
		_, err := runScript(`a = {}; a['k'] = a; print(to_json(a))`)
		So(err.(*starlark.EvalError).Msg, ShouldEqual, msg)

		// With list.
		_, err = runScript(`a = []; a.append(a); print(to_json(a))`)
		So(err.(*starlark.EvalError).Msg, ShouldEqual, msg)

		// With tuple.
		_, err = runScript(`a = []; b = (1, a); a.append(b); print(to_json(b))`)
		So(err.(*starlark.EvalError).Msg, ShouldEqual, msg)

		// And there can't be recursion involving sets, since Starlark sets require
		// all items to be hashable, and this automatically forbids containers that
		// can host recursive structures.
	})

	Convey("Bad calls", t, func() {
		_, err := runScript(`to_json()`)
		So(err.(*starlark.EvalError).Msg, ShouldEqual, "to_json: got 0 arguments, want 1")
		_, err = runScript(`to_json(1, 2, 3)`)
		So(err.(*starlark.EvalError).Msg, ShouldEqual, "to_json: got 3 arguments, want 1")
		_, err = runScript(`to_json(k=1)`)
		So(err.(*starlark.EvalError).Msg, ShouldEqual, "to_json: unexpected keyword arguments")
	})
}
