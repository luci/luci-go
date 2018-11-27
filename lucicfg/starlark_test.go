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
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarktest"

	"go.chromium.org/luci/starlark/interpreter"
)

func init() {
	// Replace DataFile implementation with non-broken one that understands GOPATH
	// with multiple entries. This is needed to pick up assert.star file under
	// Starlark package tree.
	starlarktest.DataFile = func(pkgdir, filename string) string {
		rel := filepath.Join("go.starlark.net", pkgdir, filename)
		for _, p := range build.Default.SrcDirs() {
			full := filepath.Join(p, rel)
			if _, err := os.Stat(full); err == nil {
				return full
			}
		}
		panic(fmt.Sprintf("could not find %s", rel))
	}

	// Enable not-yet-standard features.
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowFloat = true
	resolve.AllowSet = true
}

// TestAllStarlark loads and executes all test scripts (testdata/*.star).
func TestAllStarlark(t *testing.T) {
	t.Parallel()

	assertMod, err := starlarktest.LoadAssertModule()
	if err != nil {
		t.Fatalf("failed to load assertion module - %s", err)
	}

	files, err := filepath.Glob("testdata/*.star")
	if err != nil {
		t.Fatalf("failed to list *.star files - %s", err)
	}
	if len(files) == 0 {
		t.Fatalf("no *.star files in testdata, something is fishy")
	}
	sort.Strings(files)

	for _, f := range files {
		f := f
		t.Run(f, func(t *testing.T) { runSingleTest(t, f, assertMod) })
	}
}

func runSingleTest(t *testing.T, script string, predeclared starlark.StringDict) {
	ctx := context.Background()

	code, err := ioutil.ReadFile(script)
	if err != nil {
		t.Fatalf("Failed to open %q - %s", script, err)
		return
	}

	in := Inputs{
		// Make sure error messages have the original scripts name by loading the
		// test script under its true name (rather than sticking its body in
		// LUCI.star).
		Main: interpreter.MemoryLoader(map[string]string{
			"LUCI.star": fmt.Sprintf(`load("//%s", "test"); test()`, script),
			script:      string(code),
		}),

		// Expose 'assert' module, hook up error reporting to 't'.
		testPredeclared: predeclared,
		testThreadModified: func(th *starlark.Thread) {
			starlarktest.SetReporter(th, t)
			th.Print = func(_ *starlark.Thread, msg string) { t.Logf("%s", msg) }
		},
	}

	if _, err = Generate(ctx, in); err != nil {
		if evalErr, _ := err.(*starlark.EvalError); evalErr != nil {
			t.Errorf("%s\n", evalErr.Backtrace())
		} else {
			t.Errorf("%s", err)
		}
	}
}
