// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !aix || !ppc64

package generators

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/internal/testutils"
)

func TestFetchGit(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Test FetchGit generator", t, func(t *ftt.Test) {
		repoURL, commit := testutils.InitGitRepo(t)
		dir := filepath.FromSlash(strings.TrimPrefix(repoURL, "test://"))

		t.Run("Generate with commit hash", func(t *ftt.Test) {
			g := &FetchGit{
				Name:   "git-commit",
				URL:    repoURL,
				Commit: commit,
			}
			a, err := g.Generate(ctx, Platforms{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, a.Name, should.Equal("git-commit"))
			assert.Loosely(t, a.GetGit().Commit, should.Equal(commit))
			assert.Loosely(t, a.GetGit().Url, should.Equal(repoURL))
		})

		t.Run("Resolve ref", func(t *ftt.Test) {
			t.Run("tag", func(t *ftt.Test) {
				tagName := "v1.0"
				repo, err := git.PlainOpen(dir)
				assert.Loosely(t, err, should.BeNil)
				_, err = repo.CreateTag(tagName, plumbing.NewHash(commit), nil)
				assert.Loosely(t, err, should.BeNil)

				hash, err := ResolveRef(ctx, repoURL, tagName)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, hash.String(), should.Equal(commit))
			})

			t.Run("full ref", func(t *ftt.Test) {
				hash, err := ResolveRef(ctx, repoURL, "refs/heads/master")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, hash.String(), should.Equal(commit))
			})
		})
	})
}
