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

// Package starlarktest contains utilities for running Starlark tests.
//
// It knows how to run all *.star tests from some particular directory, adding
// 'assert' module to their global dict and wiring their errors to testing.T.
package starlarktest

import (
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarktest"
)

// Options describe where to discover tests and how to run them.
type Options struct {
	TestsDir    string              // directory to search for *.star files
	Predeclared starlark.StringDict // symbols to put into the global dict

	// Executor runs a single starlark test file 'path', provided as 'body'.
	//
	// If nil, RunTests will simply use starlark.ExecFile(...).
	Executor func(t *testing.T, path, body string, predeclared starlark.StringDict) error
}

// RunTests loads and executes all test scripts (testdata/**/*.star).
func RunTests(t *testing.T, opts Options) {
	assertMod, err := starlarktest.LoadAssertModule()
	if err != nil {
		t.Fatalf("failed to load assertion module - %s", err)
	}

	predecl := starlark.StringDict{}
	imp := func(m starlark.StringDict) {
		for k, v := range m {
			predecl[k] = v
		}
	}
	imp(opts.Predeclared)
	imp(assertMod)

	opts.Predeclared = predecl
	if opts.Executor == nil {
		opts.Executor = defaultExecutor
	}

	var files []string
	err = filepath.Walk(opts.TestsDir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.HasSuffix(path, ".star") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to list *.star files - %s", err)
	}
	if len(files) == 0 {
		t.Fatalf("no *.star files in %q, something is fishy", opts.TestsDir)
	}
	sort.Strings(files)

	for _, f := range files {
		f := f
		t.Run(f, func(t *testing.T) { runSingleTest(t, f, opts) })
	}
}

// HookThread makes a Starlark thread report errors and logs to the 't'.
func HookThread(th *starlark.Thread, t *testing.T) {
	starlarktest.SetReporter(th, t)
	th.Print = func(_ *starlark.Thread, msg string) { t.Logf("%s", msg) }
}

func runSingleTest(t *testing.T, script string, opts Options) {
	code, err := ioutil.ReadFile(script)
	if err != nil {
		t.Errorf("Failed to open %q - %s", script, err)
		return
	}

	// Use slash path as a script name to make stack traces look uniform across
	// OSes.
	if err := opts.Executor(t, filepath.ToSlash(script), string(code), opts.Predeclared); err != nil {
		if evalErr, _ := err.(*starlark.EvalError); evalErr != nil {
			t.Errorf("%s\n", evalErr.Backtrace())
		} else {
			t.Errorf("%s", err)
		}
	}
}

func defaultExecutor(t *testing.T, path, body string, predeclared starlark.StringDict) error {
	th := starlark.Thread{}
	HookThread(&th, t)
	_, err := starlark.ExecFile(&th, path, body, predeclared)
	return err
}

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
}
