// Copyright 2019 The LUCI Authors.
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

package typed

import (
	"testing"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/starlarktest"
)

func TestAllStarlark(t *testing.T) {
	t.Parallel()

	var (
		listLetters = []string{"T", "K", "L", "M"}
		dictLetters = []string{"K", "V", "T", "M"}
	)

	allocLetter := func(letters *[]string) (l string) {
		if len(*letters) == 0 {
			return "X"
		}
		l, *letters = (*letters)[0], (*letters)[1:]
		return
	}

	converter := func(th *starlark.Thread, cb starlark.Callable, letters *[]string) Converter {
		// Cache *callbackConverter so that all converters build from same
		// callback have same address. This is used to test list.extend fast
		// path.
		if th.Local("converters") == nil {
			th.SetLocal("converters", map[starlark.Callable]Converter{})
		}
		converters := th.Local("converters").(map[starlark.Callable]Converter)
		if converters[cb] == nil {
			converters[cb] = &callbackConverter{th, cb, allocLetter(letters)}
		}
		return converters[cb]
	}

	starlarktest.RunTests(t, starlarktest.Options{
		TestsDir: "testdata",
		Predeclared: starlark.StringDict{
			// typed_list(cb, list): new typed.List using the callback as converter.
			"typed_list": starlark.NewBuiltin("typed_list", func(th *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				var cb starlark.Callable
				var vals starlark.Value
				if err := starlark.UnpackPositionalArgs("typed_list", args, kwargs, 2, &cb, &vals); err != nil {
					return nil, err
				}
				return AsTypedList(converter(th, cb, &listLetters), vals)
			}),
			// typed_dict(key_cb, val_cb): new typed.Dict using callbacks as converters.
			"typed_dict": starlark.NewBuiltin("typed_dict", func(th *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				var keyCB starlark.Callable
				var valCB starlark.Callable
				var items starlark.Value
				if err := starlark.UnpackPositionalArgs("typed_dict", args, kwargs, 3, &keyCB, &valCB, &items); err != nil {
					return nil, err
				}
				return AsTypedDict(
					converter(th, keyCB, &dictLetters),
					converter(th, valCB, &dictLetters), items)
			}),
		},
	})
}

type callbackConverter struct {
	th  *starlark.Thread
	cb  starlark.Callable
	typ string
}

func (c *callbackConverter) Convert(x starlark.Value) (starlark.Value, error) {
	return starlark.Call(c.th, c.cb, starlark.Tuple{x}, nil)
}

func (c *callbackConverter) Type() string {
	return c.typ
}
