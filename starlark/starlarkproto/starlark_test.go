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

package starlarkproto

import (
	"os"
	"testing"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/builtins"
	"go.chromium.org/luci/starlark/starlarktest"
)

func TestAllStarlark(t *testing.T) {
	t.Parallel()

	starlarktest.RunTests(t, starlarktest.Options{
		TestsDir: "testdata",
		Predeclared: starlark.StringDict{
			"proto":  ProtoLib()["proto"],
			"struct": builtins.Struct,
			"read": starlark.NewBuiltin("read", func(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				var path string
				if err := starlark.UnpackPositionalArgs("read", args, kwargs, 1, &path); err != nil {
					return nil, err
				}
				body, err := os.ReadFile(path)
				return starlark.String(body), err
			}),
		},
	})
}
