// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLoadDefinition(t *testing.T) {
	t.Parallel()

	call := func(body string) (*Definition, error) {
		return LoadDefinition(context.Background(), []byte(deindent(body)), fakeLoaderValidator{})
	}

	t.Run("Works", func(t *testing.T) {
		pkgDef, err := call(`
			pkg.declare(name = "@pkg/name", lucicfg = "1.2.3")
			pkg.entrypoint("main.star")
			pkg.entrypoint("another/main.star")
			pkg.options.lint_checks(["none", "+formatting"])
			pkg.options.fmt_rules(
				paths = [".", "some/deeper"],
				function_args_sort = ["arg1", "arg2"],
			)
			pkg.options.fmt_rules(
				paths = ["default-sort"],
				function_args_sort = [],
			)
			pkg.options.fmt_rules(
				paths = ["noop"],
			)
			pkg.resources(["a"])
			pkg.resources(["b", "c"])
		`)
		assert.NoErr(t, err)

		// Clear stack traces to simplify comparison.
		for _, r := range pkgDef.FmtRules {
			r.Stack = nil
		}

		assert.That(t, pkgDef, should.Match(&Definition{
			Name:              "@pkg/name",
			MinLucicfgVersion: [3]int{1, 2, 3},
			Entrypoints: []string{
				"main.star",
				"another/main.star",
			},
			LintChecks: []string{"none", "+formatting"},
			FmtRules: []*FmtRule{
				{
					Paths:                 []string{".", "some/deeper"},
					SortFunctionArgs:      true,
					SortFunctionArgsOrder: []string{"arg1", "arg2"},
				},
				{
					Paths:                 []string{"default-sort"},
					SortFunctionArgs:      true,
					SortFunctionArgsOrder: []string{},
				},
				{
					Paths: []string{"noop"},
				},
			},
			Resources: []string{"a", "b", "c"},
		}))
	})

	t.Run("No load(...)", func(t *testing.T) {
		_, err := call(`load("@stdlib//builtins.star", "pkg")`)
		assert.That(t, err, should.ErrLike("cannot load @stdlib//builtins.star: load(...) is not allowed in PACKAGE.star file"))
	})

	t.Run("No exec(...)", func(t *testing.T) {
		_, err := call(`exec("@stdlib//builtins.star")`)
		assert.That(t, err, should.ErrLike("cannot exec @stdlib//builtins.star: exec(...) is not allowed in PACKAGE.star file"))
	})

	t.Run("No pkg.declare", func(t *testing.T) {
		_, err := call(`print("Hi")`)
		assert.That(t, err, should.ErrLike("PACKAGE.star must call pkg.declare(...)"))
	})

	t.Run("pkg.declare must be first", func(t *testing.T) {
		_, err := call(`pkg.entrypoint("main.star")`)
		assert.That(t, err, should.ErrLike("pkg.declare(...) must be the first statement in PACKAGE.star"))
	})

	t.Run("pkg.declare must be called once", func(t *testing.T) {
		_, err := call(`
			pkg.declare(name = "@pkg/name", lucicfg = "1.2.3")
			pkg.declare(name = "@pkg/name", lucicfg = "1.2.3")
		`)
		assert.That(t, err, should.ErrLike("pkg.declare(...) can be called at most once"))
	})

	t.Run("Bad name", func(t *testing.T) {
		cases := []struct {
			val string
			err string
		}{
			{"None", `missing required field "name"`},
			{"123", `bad "name": got int, want string`},
			{`"zzz"`, `bad package name "zzz": must start with @`},
		}
		for _, cs := range cases {
			_, err := call(fmt.Sprintf(`pkg.declare(name = %s, lucicfg = "1.2.3")`, cs.val))
			assert.That(t, err, should.ErrLike(cs.err))
		}
	})

	t.Run("Bad version", func(t *testing.T) {
		_, err := call(`pkg.declare(name = "@pkg/name", lucicfg = "1.2.3.4")`)
		assert.That(t, err, should.ErrLike(`bad lucicfg version string "1.2.3.4": expecting <major>.<minor>.<patch>`))
	})

	t.Run("Bad entrypoint", func(t *testing.T) {
		assertGenErrs(t, `
				pkg.declare(name = "@pkg/name", lucicfg = "1.2.3")
				pkg.entrypoint(%s)
			`,
			[]genErrCase{
				{"None", `missing required field "path"`},
				{`"../main.star"`, `entry point path must be within the package, got "../main.star"`},
				{`"fail.star"`, `entry point "fail.star": not passing ValidateEntrypoint`},
				{`"deeper/../main.star"`, `entry point path must be in normalized form (i.e. "main.star" instead of "deeper/../main.star")`},
			},
		)
	})

	t.Run("pkg.options.lint_checks must be called once", func(t *testing.T) {
		_, err := call(`
			pkg.declare(name = "@pkg/name", lucicfg = "1.2.3")
			pkg.options.lint_checks(["none", "+formatting"])
			pkg.options.lint_checks(["none", "+formatting"])
		`)
		assert.That(t, err, should.ErrLike("pkg.options.lint_checks(...) can be called at most once"))
	})

	t.Run("pkg.options.fmt_rules bad path", func(t *testing.T) {
		assertGenErrs(t, `
				pkg.declare(name = "@pkg/name", lucicfg = "1.2.3")
				pkg.options.fmt_rules(paths = %s)
			`,
			[]genErrCase{
				{`[None]`, `bad "paths[0]": got NoneType, want string`},
				{`[""]`, `bad "paths[0]": an empty string`},
				{`["abc/.."]`, `invalid paths: must be in normalized form (i.e. "." instead of "abc/..")`},
				{`["abc\\def"]`, `invalid paths: must be in normalized form (i.e. "abc/def" instead of "abc\\def")`},
				{`["../abc"]`, `invalid paths: must point inside the package, but got "../abc"`},
				{`["a", "a"]`, `invalid paths: "a" is specified more than once`},
			},
		)
	})

	t.Run("pkg.options.fmt_rules bad function_args_sort", func(t *testing.T) {
		assertGenErrs(t, `
				pkg.declare(name = "@pkg/name", lucicfg = "1.2.3")
				pkg.options.fmt_rules(paths = ["."], function_args_sort = %s)
			`,
			[]genErrCase{
				{`[None]`, `bad "function_args_sort[0]": got NoneType, want string`},
				{`[""]`, `bad "function_args_sort[0]": an empty string`},
				{`["a", "a"]`, `invalid function_args_sort: "a" is specified more than once`},
			},
		)
	})

	t.Run("Dup pkg.options.fmt_rules", func(t *testing.T) {
		_, err := call(`
			pkg.declare(name = "@pkg/name", lucicfg = "1.2.3")
			pkg.options.fmt_rules(
				paths = ["a", "b"],
			)
			pkg.options.fmt_rules(
				paths = ["c", "b"],
			)
		`)
		assert.That(t, err, should.ErrLike(`path "b" is already covered by an existing rule`))
	})

	t.Run("pkg.resources bad calls", func(t *testing.T) {
		assertGenErrs(t, `
				pkg.declare(name = "@pkg/name", lucicfg = "1.2.3")
				pkg.resources(%s)
			`,
			[]genErrCase{
				{`[None]`, `bad "patterns[0]": got NoneType, want string`},
				{`[""]`, `bad "patterns[0]": an empty string`},
				{`["a", "a"]`, `resource pattern "a" is declared more than once`},
			},
		)
	})
}

type genErrCase struct {
	val string
	err string
}

func assertGenErrs(t *testing.T, codeTemplate string, cases []genErrCase) {
	t.Helper()
	for _, cs := range cases {
		_, err := LoadDefinition(
			context.Background(),
			[]byte(deindent(fmt.Sprintf(codeTemplate, cs.val))),
			fakeLoaderValidator{},
		)
		assert.That(t, err, should.ErrLike(cs.err), truth.Explain("val = %s", cs.val))
	}
}

type fakeLoaderValidator struct {
	NoopLoaderValidator
}

func (fakeLoaderValidator) ValidateEntrypoint(ctx context.Context, entrypoint string) error {
	if entrypoint == "fail.star" {
		return errors.Reason("not passing ValidateEntrypoint").Err()
	}
	return nil
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
