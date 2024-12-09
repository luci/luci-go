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
	"context"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/should"

	recipepb "go.chromium.org/luci/recipes_py/proto"
)

func TestBundle(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	assert.NoErr(t, os.CopyFS(repoDir, os.DirFS(filepath.Join("testdata", "example_repo"))))
	assert.NoErr(t, filesystem.Copy(
		filepath.Join(repoDir, "recipes", ".gitignore"),
		filepath.Join(repoDir, "recipes", ".gitignore.tmpl"),
		0640))
	t.Run("comprehensive", func(t *testing.T) {
		dest := t.TempDir()
		assert.NoErr(t, Bundle(context.Background(), repoDir, dest, nil))
		assertEqualTree(t, filepath.Join("testdata", "expected_bundle"), dest)
	})

	t.Run("local override", func(t *testing.T) {
		overrideRepo, dest := t.TempDir(), t.TempDir()
		overrideCfgPath := filepath.Join(overrideRepo, filepath.Join(cfgPathSegments...))
		assert.NoErr(t, os.MkdirAll(filepath.Dir(overrideCfgPath), 0755))
		cfg, err := protojson.Marshal(&recipepb.RepoSpec{
			ApiVersion: 2,
			RepoName:   "dep_repo",
		})
		assert.NoErr(t, err)
		assert.NoErr(t, os.WriteFile(overrideCfgPath, []byte(cfg), 0644))

		assert.NoErr(t, os.MkdirAll(filepath.Join(overrideRepo, "recipes"), 0755))
		assert.NoErr(t, os.WriteFile(filepath.Join(overrideRepo, "recipes", "hi.py"), []byte("print('hi')"), 0644))
		assert.NoErr(t, Bundle(context.Background(), repoDir, dest, map[string]string{
			"dep_repo": overrideRepo,
		}))
		assertEqualTree(t, overrideRepo, filepath.Join(dest, "dep_repo"))
	})

	t.Run("repo path not provided", func(t *testing.T) {
		assert.That(t, Bundle(context.Background(), "", "whatever", nil), should.ErrLike("repoPath is required"))
	})
	t.Run("dest not provided", func(t *testing.T) {
		assert.That(t, Bundle(context.Background(), repoDir, "", nil), should.ErrLike("destination is required"))
	})
	t.Run("empty dest", func(t *testing.T) {
		dest := t.TempDir()
		f, err := os.Create(filepath.Join(dest, "foo"))
		assert.NoErr(t, err)
		defer func() { _ = f.Close() }()
		assert.That(t, Bundle(context.Background(), repoDir, dest, nil), should.ErrLike("exists but the directory is not empty"))
	})
	t.Run("unused override", func(t *testing.T) {
		assert.That(t, Bundle(context.Background(), repoDir, t.TempDir(), map[string]string{
			"unknown_dep": t.TempDir(),
		}), should.ErrLike("overrides unknown_dep provided but not used"))
	})
}

func assertEqualTree(t *testing.T, expected, actual string) {
	t.Helper()
	pathsMissingInExpected := make(stringset.Set)
	actualFS := os.DirFS(actual)
	err := fs.WalkDir(actualFS, ".", func(p string, d fs.DirEntry, err error) error {
		assert.NoErr(t, err)
		pathsMissingInExpected.Add(p)
		return nil
	})
	assert.NoErr(t, err)

	expectedFS := os.DirFS(expected)
	err = fs.WalkDir(expectedFS, ".", func(p string, d fs.DirEntry, err error) error {
		assert.NoErr(t, err)
		assert.Loosely(t, pathsMissingInExpected, should.ContainKey(p))
		pathsMissingInExpected.Del(p)

		switch actualFile := filepath.Join(actual, filepath.FromSlash(p)); {
		case d.IsDir():
			info, err := os.Stat(actualFile)
			assert.NoErr(t, err)
			assert.That(t, info.IsDir(), should.BeTrue)
		default:
			assert.That(t, d.Type().IsRegular(), should.BeTrue)
			actualFile, err := actualFS.Open(p)
			assert.NoErr(t, err)
			defer func() { _ = actualFile.Close() }()
			actualInfo, err := actualFile.Stat()
			assert.NoErr(t, err)
			assert.That(t, actualInfo.Mode().IsRegular(), should.BeTrue)
			expectedFile, err := expectedFS.Open(p)
			assert.NoErr(t, err)
			defer func() { _ = expectedFile.Close() }()
			expectedContent, err := io.ReadAll(expectedFile)
			assert.NoErr(t, err)
			actualContent, err := io.ReadAll(actualFile)
			assert.NoErr(t, err)
			assert.That(t, string(actualContent), should.Equal(string(expectedContent)))
		}
		return nil
	})
	assert.NoErr(t, err)
	assert.Loosely(t, pathsMissingInExpected,
		comparison.Func[any](should.BeEmpty), truth.LineContext())
}

func BenchmarkBundlingBuildRepo(b *testing.B) {
	tmpDir := b.TempDir()
	checkout := filepath.Join(tmpDir, "checkout")
	_, err := git.PlainClone(checkout, false, &git.CloneOptions{
		URL:           "https://chromium.googlesource.com/chromium/tools/build",
		ReferenceName: plumbing.ReferenceName("refs/heads/main"),
		SingleBranch:  true,
		Depth:         1,
	})
	assert.NoErr(b, err)

	// this will download all the dependencies.
	cmd := exec.Command(filepath.Join("recipes", "recipes.py"), "fetch")
	cmd.Dir = checkout
	assert.NoErr(b, cmd.Run())
	b.ResetTimer()
	for range b.N {
		bundleDir, err := os.MkdirTemp(tmpDir, "bundle_*")
		assert.NoErr(b, err)
		assert.NoErr(b, Bundle(context.Background(), checkout, bundleDir, nil))
	}
}
