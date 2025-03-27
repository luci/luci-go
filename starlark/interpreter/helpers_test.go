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

package interpreter

// Code shared by tests.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"unicode"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/starlark/builtins"
)

const (
	// Name of the main package used by tests.
	mainPkg = "test_main"
	// Name of the stdlib package used by tests.
	stdlibPkg = "stdlib"
)

// deindent finds first non-empty and non-whitespace line and subtracts its
// indentation from all lines.
func deindent(s string) string {
	lines := strings.Split(s, "\n")

	indent := ""
	for _, line := range lines {
		idx := strings.IndexFunc(line, func(r rune) bool {
			return !unicode.IsSpace(r)
		})
		if idx != -1 {
			indent = line[:idx]
			break
		}
	}

	if indent == "" {
		return s
	}

	trimmed := make([]string, len(lines))
	for i, line := range lines {
		trimmed[i] = strings.TrimPrefix(line, indent)
	}
	return strings.Join(trimmed, "\n")
}

// deindentLoader deindents starlark code before returning it.
func deindentLoader(files map[string]string) Loader {
	return func(_ context.Context, path string) (_ starlark.StringDict, src string, err error) {
		body, ok := files[path]
		if !ok {
			return nil, "", ErrNoModule
		}
		return nil, deindent(body), nil
	}
}

func TestDeindent(t *testing.T) {
	t.Parallel()
	ftt.Run("Works", t, func(t *ftt.Test) {
		s := deindent(`

		a
			b
				c
			d

		e
		`)
		assert.Loosely(t, s, should.Match(`

a
	b
		c
	d

e
`))
	})
}

// intrParams are arguments for runIntr helper.
type intrParams struct {
	ctx context.Context

	// scripts contains user-supplied scripts (ones that would normally be loaded
	// from the file system). If there's main.star script, it will be executed via
	// LoadModule and its global dict keys returned.
	scripts map[string]string

	// stdlib contains stdlib source code, as path => body mapping. In particular,
	// builtins.star will be auto-loaded by the interpreter.
	stdlib map[string]string

	// package 'custom' is used by tests for Loaders.
	custom Loader

	predeclared  starlark.StringDict
	preExec      func(th *starlark.Thread, module ModuleKey)
	postExec     func(th *starlark.Thread, module ModuleKey)
	checkVisible func(th *starlark.Thread, loader, loaded ModuleKey) error

	forbidLoad string
	forbidExec string

	visited *[]ModuleKey
}

// runIntr initializes and runs the interpreter over given scripts, by loading
// main.star using ExecModule.
//
// Returns keys of the dict of the main.star script (if any), and a list of
// messages logged via print(...).
func runIntr(p intrParams) (keys []string, logs []string, err error) {
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	intr := Interpreter{
		Predeclared: p.predeclared,
		PreExec:     p.preExec,
		PostExec:    p.postExec,
		Packages: map[string]Loader{
			mainPkg:   deindentLoader(p.scripts),
			stdlibPkg: deindentLoader(p.stdlib),
			"custom":  p.custom,
		},
		MainPackage: mainPkg,
		Logger: func(file string, line int, message string) {
			logs = append(logs, fmt.Sprintf("[%s:%d] %s", file, line, message))
		},
		ForbidLoad:   p.forbidLoad,
		ForbidExec:   p.forbidExec,
		CheckVisible: p.checkVisible,
	}

	// Load builtins if the test actually defined them.
	err = intr.LoadBuiltins(ctx, stdlibPkg, "builtins.star")
	if err != nil && err != ErrNoModule {
		return
	}

	if _, ok := p.scripts["main.star"]; ok {
		var dict starlark.StringDict
		dict, err = intr.ExecModule(ctx, mainPkg, "main.star")
		if err == nil {
			keys = make([]string, 0, len(dict))
			for k := range dict {
				keys = append(keys, k)
			}
			sort.Strings(keys)
		}
	}

	if p.visited != nil {
		*p.visited = intr.Visited()
	}

	return
}

// normalizeErr takes an error and extracts a normalized stack trace from it.
func normalizeErr(err error) string {
	if err == nil {
		return ""
	}
	if evalErr, ok := err.(*starlark.EvalError); ok {
		return builtins.NormalizeStacktrace(evalErr.Backtrace())
	}
	return builtins.NormalizeStacktrace(err.Error())
}
