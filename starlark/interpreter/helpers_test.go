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
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"

	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	// Enable not-yet-standard features.
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowFloat = true
	resolve.AllowSet = true
}

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

func TestDeindent(t *testing.T) {
	t.Parallel()
	Convey("Works", t, func() {
		s := deindent(`

		a
			b
				c
			d

		e
		`)
		So(s, ShouldResemble, `

a
	b
		c
	d

e
`)
	})
}

// intrParams are arguments for runIntr helper.
type intrParams struct {
	// scripts contains user-supplied scripts (ones that would normally be loaded
	// from the file system). If there's main.star script, it will be executed via
	// ExecFile and its global dict keys returned.
	scripts map[string]string

	// stdlib contains stdlib source code, as path => body mapping. In particular,
	// init.star will be auto-loaded by the interpreter.
	stdlib map[string]string

	// predeclared are just passed directly to the interpreter.
	predeclared starlark.StringDict
}

// runIntr initializes and runs the interpreter over given scripts.
//
// Returns keys of the dict of the main.star script (if any), and a list of
// messages logged via print(...).
func runIntr(p intrParams) (keys []string, logs []string, err error) {
	intr := Interpreter{
		Predeclared: p.predeclared,
		Usercode: func(path string) (string, error) {
			if body, ok := p.scripts[filepath.ToSlash(path)]; ok {
				return deindent(body), nil
			}
			return "", fmt.Errorf("no such script file %s", path)
		},
		Stdlib: func(path string) (string, error) {
			if body, ok := p.stdlib[path]; ok {
				return deindent(body), nil
			}
			return "", ErrNoStdlibModule
		},
		Logger: func(file string, line int, message string) {
			logs = append(logs, fmt.Sprintf("[%s:%d] %s", file, line, message))
		},
	}

	if err = intr.Init(); err != nil {
		return
	}

	if _, ok := p.scripts["main.star"]; ok {
		var dict starlark.StringDict
		dict, err = intr.ExecFile("main.star")
		if err == nil {
			keys = make([]string, 0, len(dict))
			for k := range dict {
				keys = append(keys, k)
			}
			sort.Strings(keys)
		}
	}

	return
}

// runScript runs a single init.star script through the interpreter.
func runScript(body string) error {
	_, _, err := runIntr(intrParams{
		stdlib: map[string]string{"init.star": body},
	})
	return err
}
