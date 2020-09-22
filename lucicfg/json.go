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

package lucicfg

import (
	"encoding/json"
	"fmt"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/builtins"
)

// toSortedJSON is to_json(value) builtin.
//
// It historically preceded json.encode(...). Unlike json.encode(...) it emits
// JSON with dict keys sorted.
var toSortedJSON = starlark.NewBuiltin("to_json", func(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var v starlark.Value
	if err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &v); err != nil {
		return nil, err
	}
	obj, err := builtins.ToGoNative(v)
	if err != nil {
		return nil, fmt.Errorf("to_json: %s", err)
	}
	blob, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("to_json: %s", err)
	}
	return starlark.String(blob), nil
})
