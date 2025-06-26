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

import (
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// runs a script in an environment where 'custom' package uses the given loader.
func runScriptWithLoader(body string, l Loader) (logs []string, err error) {
	_, logs, err = runIntr(intrParams{
		stdlib: map[string]string{"builtins.star": body},
		custom: l,
	})
	return logs, err
}

func TestLoaders(t *testing.T) {
	t.Parallel()

	ftt.Run("FileSystemLoader", t, func(t *ftt.Test) {
		tmp, err := os.MkdirTemp("", "starlark")
		assert.Loosely(t, err, should.BeNil)
		defer os.RemoveAll(tmp)

		loader := FileSystemLoader(tmp)

		put := func(path, body string) {
			path = filepath.Join(tmp, filepath.FromSlash(path))
			assert.Loosely(t, os.MkdirAll(filepath.Dir(path), 0700), should.BeNil)
			assert.Loosely(t, os.WriteFile(path, []byte(body), 0600), should.BeNil)
		}

		t.Run("Works", func(t *ftt.Test) {
			put("1.star", `load("//a/b/c/2.star", _sym="sym"); sym = _sym`)
			put("a/b/c/2.star", "print('Hi')\nsym = 1")

			logs, err := runScriptWithLoader(`load("@custom//1.star", "sym")`, loader)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, logs, should.Match([]string{
				"[@custom//a/b/c/2.star:1] Hi",
			}))
		})

		t.Run("Missing module", func(t *ftt.Test) {
			put("1.star", `load("//a/b/c/2.star", "sym")`)

			_, err := runScriptWithLoader(`load("@custom//1.star", "sym")`, loader)
			assert.Loosely(t, err, should.ErrLike("cannot load //a/b/c/2.star: no such module"))
		})

		t.Run("Outside the root", func(t *ftt.Test) {
			_, err := runScriptWithLoader(`load("@custom//../1.star", "sym")`, loader)
			assert.Loosely(t, err, should.ErrLike("cannot load @custom//../1.star: outside the package root"))
		})
	})

	ftt.Run("MemoryLoader", t, func(t *ftt.Test) {
		t.Run("Works", func(t *ftt.Test) {
			loader := MemoryLoader(map[string]string{
				"1.star":       `load("//a/b/c/2.star", _sym="sym"); sym = _sym`,
				"a/b/c/2.star": "print('Hi')\nsym = 1",
			})

			logs, err := runScriptWithLoader(`load("@custom//1.star", "sym")`, loader)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, logs, should.Match([]string{
				"[@custom//a/b/c/2.star:1] Hi",
			}))
		})

		t.Run("Missing module", func(t *ftt.Test) {
			loader := MemoryLoader(map[string]string{
				"1.star": `load("//a/b/c/2.star", "sym")`,
			})

			_, err := runScriptWithLoader(`load("@custom//1.star", "sym")`, loader)
			assert.Loosely(t, err, should.ErrLike("cannot load //a/b/c/2.star: no such module"))
		})
	})
}
