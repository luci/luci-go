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

package lucicfg

import (
	"context"
	"testing"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/interpreter"
	"go.chromium.org/luci/starlark/starlarktest"
)

func init() {
	// Enable not-yet-standard features.
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowFloat = true
	resolve.AllowSet = true
}

// TestAllStarlark loads and executes all test scripts (testdata/*.star).
func TestAllStarlark(t *testing.T) {
	t.Parallel()

	starlarktest.RunTests(t, starlarktest.Options{
		TestsDir: "testdata",

		Executor: func(t *testing.T, path, body string, predeclared starlark.StringDict) error {
			_, err := Generate(context.Background(), Inputs{
				// Make sure error messages have the original scripts name by loading the
				// test script under its true name.
				Code:  interpreter.MemoryLoader(map[string]string{path: body}),
				Entry: path,

				// Expose 'assert' module, hook up error reporting to 't'.
				testPredeclared: predeclared,
				testThreadModified: func(th *starlark.Thread) {
					starlarktest.HookThread(th, t)
				},
			})
			return err
		},
	})
}
