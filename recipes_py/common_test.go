// Copyright 2024 The LUCI Authors.
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

package recipespy

import (
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	recipepb "go.chromium.org/luci/recipes_py/proto"
)

func TestRepoFromPath(t *testing.T) {
	t.Run("happy case", func(t *testing.T) {
		t.Parallel()
		repoDir := t.TempDir()
		const cfgContent = `{
		"api_version": 2,
		"repo_name": "example",
		"recipes_path": "recipes"
}`
		cfgPath := filepath.Join(repoDir, filepath.Join(cfgPathSegments...))
		assert.NoErr(t, os.MkdirAll(filepath.Dir(cfgPath), 0755))
		assert.NoErr(t, os.WriteFile(cfgPath, []byte(cfgContent), 0644))

		repo, err := RepoFromPath(repoDir)
		assert.NoErr(t, err)
		absRepoDir, err := filepath.Abs(repoDir)
		assert.NoErr(t, err)
		assert.That(t, repo.Path, should.Equal(absRepoDir))
		assert.That(t, repo.Spec, should.Match(&recipepb.RepoSpec{
			ApiVersion:  2,
			RepoName:    "example",
			RecipesPath: "recipes",
		}))
	})

	t.Run("missing recipes.cfg", func(t *testing.T) {
		t.Parallel()
		repoDir := t.TempDir()
		repo, err := RepoFromPath(repoDir)
		assert.That(t, err, should.ErrLike("failed to read recipes.cfg"))
		assert.Loosely(t, repo, should.BeNil)
	})

	t.Run("missing recipe repo", func(t *testing.T) {
		t.Parallel()
		repoDir := t.TempDir()
		assert.NoErr(t, os.Remove(repoDir))
		repo, err := RepoFromPath(repoDir)
		assert.That(t, err, should.ErrLike("failed to read recipes.cfg"))
		assert.Loosely(t, repo, should.BeNil)
	})
}
