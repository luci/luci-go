// Copyright 2020 The LUCI Authors.
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

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	resolve.AllowFloat = true
	resolve.AllowSet = true
}

func TestToGoNative(t *testing.T) {
	t.Parallel()

	runScript := func(code string) (starlark.StringDict, error) {
		return starlark.ExecFile(&starlark.Thread{}, "main", code, nil)
	}

	run := func(code string, expected any) {
		out, err := runScript(code)
		So(err, ShouldBeNil)
		result, err := ToGoNative(out["val"])
		So(err, ShouldBeNil)
		So(result, ShouldResemble, expected)
	}

	mustFail := func(code string, expectErr string) {
		out, err := runScript(code)
		_, err = ToGoNative(out["val"])
		So(err, ShouldErrLike, expectErr)
	}

	Convey("Happy cases", t, func() {
		Convey("Scalar types", func() {
			run(`val = None`, nil)
			run(`val = True`, true)
			run(`val = 123`, int64(123))
			run(`val = 123.5`, 123.5)
			run(`val = "hi"`, "hi")
		})

		Convey("Dict", func() {
			run(`val = {}`, map[string]any{})
			run(`val = {"a": 1, "b": 2}`, map[string]any{"a": int64(1), "b": int64(2)})
		})

		Convey("Iterables", func() {
			run(`val = []`, []any{})
			run(`val = ()`, []any{})
			run(`val = set()`, []any{})
			run(`val = [1, 2, 3]`, []any{int64(1), int64(2), int64(3)})
			run(`val = (1, 2, 3)`, []any{int64(1), int64(2), int64(3)})
			run(`val = set([1, 2, 3])`, []any{int64(1), int64(2), int64(3)})
		})

		Convey("Everything at once", func() {
			run(`val = {"a": None, "b": ["c", "d", ["e"]]}`, map[string]any{
				"a": nil,
				"b": []any{"c", "d", []any{"e"}},
			})
		})
	})

	Convey("Unhappy cases", t, func() {
		mustFail(`val = list`, `unsupported type builtin_function_or_method`)
		mustFail(`val = 18446744073709551616`, `can't convert "18446744073709551616" to int64`)
		mustFail(`val = {1: 2}`, `dict keys should be strings, got int`)

		// Errors propagate when serializing containers.
		mustFail(`val = {"a": list}`, `unsupported type builtin_function_or_method`)
		mustFail(`val = [list]`, `unsupported type builtin_function_or_method`)
		mustFail(`val = (list, list)`, `unsupported type builtin_function_or_method`)
		mustFail(`val = set([list])`, `unsupported type builtin_function_or_method`)
	})

	Convey("Detects recursive structures", t, func() {
		const msg = "detected recursion in the data structure"

		// With dict.
		mustFail(`val = {}; val['k'] = val`, msg)
		// With list.
		mustFail(`val = []; val.append(val)`, msg)
		// With tuple.
		mustFail(`l = []; val = (1, l); l.append(val)`, msg)

		// And there can't be recursion involving sets, since Starlark sets require
		// all items to be hashable, and this automatically forbids containers that
		// can host recursive structures.
	})
}
