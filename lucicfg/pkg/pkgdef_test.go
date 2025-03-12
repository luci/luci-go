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
			pkg.declare(name = "pkg/name", lucicfg = "1.2.3")
			pkg.entrypoint("main.star")
			pkg.entrypoint("another/main.star")
			pkg.options.lint_checks(["none", "+formatting"])
		`)
		assert.NoErr(t, err)
		assert.That(t, pkgDef, should.Match(&Definition{
			Name:              "pkg/name",
			MinLucicfgVersion: [3]int{1, 2, 3},
			Entrypoints: []string{
				"main.star",
				"another/main.star",
			},
			LintChecks: []string{"none", "+formatting"},
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
			pkg.declare(name = "pkg/name", lucicfg = "1.2.3")
			pkg.declare(name = "pkg/name", lucicfg = "1.2.3")
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
			{`"//"`, `bad package name "//": empty path component`},
		}
		for _, cs := range cases {
			_, err := call(fmt.Sprintf(`pkg.declare(name = %s, lucicfg = "1.2.3")`, cs.val))
			assert.That(t, err, should.ErrLike(cs.err))
		}
	})

	t.Run("Bad version", func(t *testing.T) {
		_, err := call(`pkg.declare(name = "pkg/name", lucicfg = "1.2.3.4")`)
		assert.That(t, err, should.ErrLike(`bad lucicfg version string "1.2.3.4": expecting <major>.<minor>.<patch>`))
	})

	t.Run("Bad entrypoint", func(t *testing.T) {
		cases := []struct {
			val string
			err string
		}{
			{"None", `missing required field "path"`},
			{`"../main.star"`, `entry point path must be within the package, got "../main.star"`},
			{`"fail.star"`, `entry point "fail.star": not passing ValidateEntrypoint`},
			{`"deeper/../main.star"`, `entry point path must be in normalized form (i.e. "main.star" instead of "deeper/../main.star")`},
		}
		for _, cs := range cases {
			_, err := call(fmt.Sprintf(`
				pkg.declare(name = "pkg/name", lucicfg = "1.2.3")
				pkg.entrypoint(%s)
			`, cs.val))
			assert.That(t, err, should.ErrLike(cs.err))
		}
	})

	t.Run("pkg.options.lint_checks must be called once", func(t *testing.T) {
		_, err := call(`
			pkg.declare(name = "pkg/name", lucicfg = "1.2.3")
			pkg.options.lint_checks(["none", "+formatting"])
			pkg.options.lint_checks(["none", "+formatting"])
		`)
		assert.That(t, err, should.ErrLike("pkg.options.lint_checks(...) can be called at most once"))
	})
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
