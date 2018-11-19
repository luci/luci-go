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

package starlarkproto

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarktest"

	// Register proto types in the protobuf lib registry.
	_ "go.chromium.org/luci/starlark/starlarkproto/testprotos"
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

	predeclared := starlark.StringDict{}
	importMod := func(m starlark.StringDict) {
		for k, v := range m {
			predeclared[k] = v
		}
	}

	importMod(assertMod)
	importMod(ProtoLib())

	// Enumerate all test cases.
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
		t.Run(f, func(t *testing.T) { runSingleTest(t, f, predeclared) })
	}
}

func runSingleTest(t *testing.T, script string, predeclared starlark.StringDict) {
	thread := starlark.Thread{
		Load: func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
			if strings.HasSuffix(module, ".proto") {
				return LoadProtoModule(module)
			}
			return nil, fmt.Errorf("don't know how to load starlark module %q", module)
		},
		Print: func(th *starlark.Thread, msg string) { t.Logf("%s", msg) },
	}
	starlarktest.SetReporter(&thread, t)
	_, err := starlark.ExecFile(&thread, script, nil, predeclared)
	if !t.Failed() && err != nil {
		t.Errorf("Script execution failed - %s", err)
	}
}
