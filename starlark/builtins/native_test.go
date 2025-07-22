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

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestToGoNative(t *testing.T) {
	t.Parallel()

	runScript := func(code string) (starlark.StringDict, error) {
		return starlark.ExecFileOptions(&syntax.FileOptions{
			Set: true,
		}, &starlark.Thread{}, "main", code, nil)
	}

	run := func(t testing.TB, code string, expected any) {
		t.Helper()

		out, err := runScript(code)
		assert.NoErr(t, err, truth.LineContext())
		result, err := ToGoNative(out["val"])
		assert.NoErr(t, err, truth.LineContext())
		assert.Loosely(t, result, should.Match(expected), truth.LineContext())
	}

	mustFail := func(code string, expectErr string) {
		out, err := runScript(code)
		assert.Loosely(t, err, should.ErrLike(nil))
		_, err = ToGoNative(out["val"])
		assert.Loosely(t, err, should.ErrLike(expectErr))
	}

	ftt.Run("Happy cases", t, func(t *ftt.Test) {
		t.Run("Scalar types", func(t *ftt.Test) {
			run(t, `val = None`, nil)
			run(t, `val = True`, true)
			run(t, `val = 123`, int64(123))
			run(t, `val = 123.5`, 123.5)
			run(t, `val = "hi"`, "hi")
		})

		t.Run("Dict", func(t *ftt.Test) {
			run(t, `val = {}`, map[string]any{})
			run(t, `val = {"a": 1, "b": 2}`, map[string]any{"a": int64(1), "b": int64(2)})
		})

		t.Run("Iterables", func(t *ftt.Test) {
			run(t, `val = []`, []any{})
			run(t, `val = ()`, []any{})
			run(t, `val = set()`, []any{})
			run(t, `val = [1, 2, 3]`, []any{int64(1), int64(2), int64(3)})
			run(t, `val = (1, 2, 3)`, []any{int64(1), int64(2), int64(3)})
			run(t, `val = set([1, 2, 3])`, []any{int64(1), int64(2), int64(3)})
		})

		t.Run("Everything at once", func(t *ftt.Test) {
			run(t, `val = {"a": None, "b": ["c", "d", ["e"]]}`, map[string]any{
				"a": nil,
				"b": []any{"c", "d", []any{"e"}},
			})
		})
	})

	ftt.Run("Unhappy cases", t, func(t *ftt.Test) {
		mustFail(`val = list`, `unsupported type builtin_function_or_method`)
		mustFail(`val = 18446744073709551616`, `can't convert "18446744073709551616" to int64`)
		mustFail(`val = {1: 2}`, `dict keys should be strings, got int`)

		// Errors propagate when serializing containers.
		mustFail(`val = {"a": list}`, `unsupported type builtin_function_or_method`)
		mustFail(`val = [list]`, `unsupported type builtin_function_or_method`)
		mustFail(`val = (list, list)`, `unsupported type builtin_function_or_method`)
		mustFail(`val = set([list])`, `unsupported type builtin_function_or_method`)
	})

	ftt.Run("Detects recursive structures", t, func(t *ftt.Test) {
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
