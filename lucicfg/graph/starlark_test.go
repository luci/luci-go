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

package graph

import (
	"testing"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/builtins"
	"go.chromium.org/luci/starlark/starlarktest"
)

func TestGraph(t *testing.T) {
	t.Parallel()

	starlarktest.RunTests(t, starlarktest.Options{
		TestsDir: "testdata",

		Predeclared: starlark.StringDict{
			"stacktrace": builtins.Stacktrace,
			"new_graph": starlark.NewBuiltin("new_graph", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
				return &Graph{}, nil
			}),
		},
	})
}
