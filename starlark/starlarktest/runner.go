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
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarktest"
	"go.starlark.net/syntax"

	"go.chromium.org/luci/starlark/starlarktest/vendored"
)

// Options describe where to discover tests and how to run them.
type Options struct {
	TestsDir    string              // directory to search for *.star files
	Skip        string              // directories with this name are skipped
	Predeclared starlark.StringDict // symbols to put into the global dict
	FileOptions *syntax.FileOptions // passed to the Starlark interpreter

	// Executor runs a single starlark test file.
	//
	// If nil, RunTests will simply use starlark.ExecFile(...).
	Executor func(t *testing.T, path string, predeclared starlark.StringDict) error
}

// RunTests loads and executes all test scripts (testdata/**/*.star).
func RunTests(t *testing.T, opts Options) {
	cleanup, err := materializeAssertStar()
	if err != nil {
		t.Fatalf("failed to load assertion module - %s", err)
	}
	defer cleanup()

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
		if opts.FileOptions == nil {
			opts.FileOptions = &syntax.FileOptions{Set: true}
		}
		opts.Executor = makeDefaultExecutor(opts.FileOptions)
	}

	var files []string
	err = filepath.Walk(opts.TestsDir, func(path string, info os.FileInfo, err error) error {
		switch {
		case info.IsDir() && info.Name() == opts.Skip:
			return filepath.SkipDir
		case !info.IsDir() && strings.HasSuffix(path, ".star"):
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
		t.Run(f, func(t *testing.T) { runSingleTest(t, f, opts) })
	}
}

// HookThread makes a Starlark thread report errors and logs to the 't'.
func HookThread(th *starlark.Thread, t *testing.T) {
	starlarktest.SetReporter(th, t)
	th.Print = func(_ *starlark.Thread, msg string) { t.Logf("%s", msg) }
}

func runSingleTest(t *testing.T, script string, opts Options) {
	if err := opts.Executor(t, script, opts.Predeclared); err != nil {
		if evalErr, _ := err.(*starlark.EvalError); evalErr != nil {
			t.Errorf("%s\n", evalErr.Backtrace())
		} else {
			t.Errorf("%s", err)
		}
	}
}

func makeDefaultExecutor(opts *syntax.FileOptions) func(*testing.T, string, starlark.StringDict) error {
	return func(t *testing.T, path string, predeclared starlark.StringDict) error {
		th := starlark.Thread{}
		HookThread(&th, t)

		code, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Use slash path as a script name to make stack traces look uniform across
		// OSes.
		_, err = starlark.ExecFileOptions(&syntax.FileOptions{Set: true}, &th, filepath.ToSlash(path), code, predeclared)
		return err
	}
}

// materializeAssertStar creates assert.star file on disk to feed it to the
// go.starlark.net/starlarktest package, since it wants a real file.
//
// It is unable to discover its own copy of assert.star when running in Modules
// mode.
func materializeAssertStar() (cleanup func(), err error) {
	tmp, err := ioutil.TempDir("", "starlarktest")
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(
		filepath.Join(tmp, "assert.star"),
		vendored.GetAsset("starlarktest/assert.star"),
		0600)
	if err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}

	prev := starlarktest.DataFile
	starlarktest.DataFile = func(pkgdir, filename string) string {
		path := fmt.Sprintf("%s/%s", pkgdir, filename)
		if path == "starlarktest/assert.star" {
			return filepath.Join(tmp, "assert.star")
		}
		panic("don't know how to load " + path)
	}

	return func() {
		starlarktest.DataFile = prev
		os.RemoveAll(tmp)
	}, nil
}
