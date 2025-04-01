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

package gitsource

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCache(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		tdir := t.TempDir()
		cachePath := filepath.Join(tdir, "cache")

		_, err := New(cachePath)
		assert.NoErr(t, err)

		finfo, err := os.Stat(cachePath)
		assert.NoErr(t, err)
		assert.That(t, finfo.IsDir(), should.BeTrue)
	})

	t.Run("not abs", func(t *testing.T) {
		t.Parallel()

		_, err := New("hello")
		assert.ErrIsLike(t, err, "not absolute")
	})

	t.Run("unclean path", func(t *testing.T) {
		t.Parallel()

		pth := "/hello/../world"
		if runtime.GOOS == "windows" {
			pth = "c:" + pth
		}

		_, err := New(pth)
		assert.ErrIsLike(t, err, "not clean")
	})
}

func TestCacheForRepo(t *testing.T) {
	t.Parallel()

	tempRepo := mkRepoRaw(t)

	t.Run("ok", func(t *testing.T) {
		sharedCacheDir := t.TempDir()

		cache, err := New(sharedCacheDir)
		assert.NoErr(t, err)

		ctx := context.Background()

		repo, err := cache.ForRepo(ctx, tempRepo)
		assert.NoErr(t, err)

		files, err := os.ReadDir(cache.cacheRoot)
		assert.NoErr(t, err)

		check.Loosely(t, files, should.HaveLength(1))
		check.That(t, files[0].Name(), should.Equal(filepath.Base(repo.repoRoot)))

		t.Run("identical second lookup", func(t *testing.T) {
			repo2, err := cache.ForRepo(ctx, tempRepo)
			assert.NoErr(t, err)
			assert.That(t, repo, should.Equal(repo2)) // should be identical
		})

		t.Run("second cache, existing repo", func(t *testing.T) {
			cache2, err := New(sharedCacheDir)
			assert.NoErr(t, err)

			repo2, err := cache2.ForRepo(ctx, tempRepo)
			assert.NoErr(t, err)

			assert.That(t, repo2.repoRoot, should.Equal(repo.repoRoot))
		})
	})

	t.Run("bad cache", func(t *testing.T) {
		_, err := (&Cache{}).ForRepo(context.Background(), "does not matter")
		assert.ErrIsLike(t, err, "must be constructed")
	})
}
