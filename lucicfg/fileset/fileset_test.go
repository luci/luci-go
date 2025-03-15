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

package fileset

import (
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestScanDirectory(t *testing.T) {
	t.Parallel()

	ftt.Run("With a bunch of files", t, func(t *ftt.Test) {
		tmp := t.TempDir()

		touch := func(p string) {
			p = filepath.Join(tmp, filepath.FromSlash(p))
			assert.NoErr(t, os.MkdirAll(filepath.Dir(p), 0700))
			assert.NoErr(t, os.WriteFile(p, nil, 0600))
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
			set, err := New([]string{"*.cfg", "sub/*", "!**/*-dev.cfg"})
			assert.NoErr(t, err)
			files, err := ScanDirectory(tmp, set)
			assert.NoErr(t, err)
			assert.That(t, files, should.Match([]string{"a.cfg", "sub/a.cfg"}))
		})

		t.Run("No negative", func(t *ftt.Test) {
			set, err := New([]string{"**/*.cfg"})
			assert.NoErr(t, err)
			files, err := ScanDirectory(tmp, set)
			assert.NoErr(t, err)
			assert.That(t, files, should.Match([]string{
				"a-dev.cfg",
				"a.cfg",
				"sub/a-dev.cfg",
				"sub/a.cfg",
			}))
		})

		t.Run("No positive", func(t *ftt.Test) {
			set, err := New([]string{"!**/*.cfg"})
			assert.NoErr(t, err)
			files, err := ScanDirectory(tmp, set)
			assert.NoErr(t, err)
			assert.Loosely(t, files, should.BeEmpty)
		})

		t.Run("Implied **/*", func(t *ftt.Test) {
			set, err := New([]string{"!**/*-dev.cfg"})
			assert.NoErr(t, err)
			files, err := ScanDirectory(tmp, set)
			assert.NoErr(t, err)
			assert.That(t, files, should.Match([]string{
				"a.cfg",
				"sub/a.cfg",
			}))
		})

		t.Run("Missing directory", func(t *ftt.Test) {
			set, err := New([]string{"*.cfg"})
			assert.NoErr(t, err)
			files, err := ScanDirectory(filepath.Join(tmp, "missing"), set)
			assert.NoErr(t, err)
			assert.Loosely(t, files, should.BeEmpty)
		})

		t.Run("Empty patterns", func(t *ftt.Test) {
			set, err := New(nil)
			assert.NoErr(t, err)
			files, err := ScanDirectory(tmp, set)
			assert.NoErr(t, err)
			assert.Loosely(t, files, should.BeEmpty)
		})

		t.Run("Bad positive pattern", func(t *ftt.Test) {
			set, err := New([]string{"["})
			assert.NoErr(t, err)
			_, err = ScanDirectory(tmp, set)
			assert.That(t, err, should.ErrLike(`bad pattern "["`))
		})

		t.Run("Bad negative pattern", func(t *ftt.Test) {
			set, err := New([]string{"*", "!["})
			assert.NoErr(t, err)
			_, err = ScanDirectory(tmp, set)
			assert.That(t, err, should.ErrLike(`bad pattern "["`))
		})
	})
}
