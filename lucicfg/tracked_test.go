// Copyright 2019 The LUCI Authors.
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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFindTrackedFiles(t *testing.T) {
	t.Parallel()

	ftt.Run("With a bunch of files", t, func(t *ftt.Test) {
		tmp, err := ioutil.TempDir("", "lucicfg")
		assert.Loosely(t, err, should.BeNil)
		defer os.RemoveAll(tmp)

		touch := func(p string) {
			p = filepath.Join(tmp, filepath.FromSlash(p))
			assert.Loosely(t, os.MkdirAll(filepath.Dir(p), 0700), should.BeNil)
			assert.Loosely(t, os.WriteFile(p, nil, 0600), should.BeNil)
		}

		files := []string{
			"a-dev.cfg",
			"a.cfg",
			"sub/a-dev.cfg",
			"sub/a.cfg",
		}
		for _, f := range files {
			touch(f)
		}

		t.Run("Works", func(t *ftt.Test) {
			files, err := FindTrackedFiles(tmp, []string{"*.cfg", "sub/*", "!**/*-dev.cfg"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, files, should.Resemble([]string{"a.cfg", "sub/a.cfg"}))
		})

		t.Run("No negative", func(t *ftt.Test) {
			files, err := FindTrackedFiles(tmp, []string{"**/*.cfg"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, files, should.Resemble([]string{
				"a-dev.cfg",
				"a.cfg",
				"sub/a-dev.cfg",
				"sub/a.cfg",
			}))
		})

		t.Run("No positive", func(t *ftt.Test) {
			files, err := FindTrackedFiles(tmp, []string{"!**/*.cfg"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, files, should.HaveLength(0))
		})

		t.Run("Implied **/*", func(t *ftt.Test) {
			files, err := FindTrackedFiles(tmp, []string{"!**/*-dev.cfg"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, files, should.Resemble([]string{
				"a.cfg",
				"sub/a.cfg",
			}))
		})

		t.Run("Missing directory", func(t *ftt.Test) {
			files, err := FindTrackedFiles(filepath.Join(tmp, "missing"), []string{"*.cfg"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, files, should.HaveLength(0))
		})

		t.Run("Empty patterns", func(t *ftt.Test) {
			files, err := FindTrackedFiles(tmp, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, files, should.HaveLength(0))
		})

		t.Run("Bad positive pattern", func(t *ftt.Test) {
			_, err := FindTrackedFiles(tmp, []string{"["})
			assert.Loosely(t, err, should.ErrLike(`bad pattern "["`))
		})

		t.Run("Bad negative pattern", func(t *ftt.Test) {
			_, err := FindTrackedFiles(tmp, []string{"*", "!["})
			assert.Loosely(t, err, should.ErrLike(`bad pattern "["`))
		})
	})
}
