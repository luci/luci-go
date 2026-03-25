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

package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/server"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
)

func TestProcessGit(t *testing.T) {
	ftt.Run("Test action processor for git", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		gitSpec := &core.ActionGitFetch{
			Url:    "https://host.not.exist/repo",
			Commit: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		}

		pkg, err := ap.Process("", pm, &core.Action{
			Name: "git",
			Spec: &core.Action_Git{Git: gitSpec},
		})
		assert.Loosely(t, err, should.BeNil)

		checkReexecArg(t, pkg.Derivation.Args, gitSpec)
	})
}

func TestExecuteGit(t *testing.T) {
	ftt.Run("Test execute action git", t, func(t *ftt.Test) {
		ctx := context.Background()
		repo, commit := initGitRepo(t)
		out := t.TempDir()

		t.Run("Test clone and checkout commit", func(t *ftt.Test) {
			a := &core.ActionGitFetch{
				Url:    repo,
				Commit: commit,
			}

			err := ActionGitFetchExecutor(ctx, a, out)
			assert.Loosely(t, err, should.BeNil)

			// Verify the repo was cloned and checked out
			fetched, err := git.PlainOpen(out)
			assert.Loosely(t, err, should.BeNil)
			head, err := fetched.Head()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, head.Hash().String(), should.Equal(commit))
		})
	})
}

func TestReexecGit(t *testing.T) {
	ftt.Run("Test re-execute action processor for git", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")
		ctx := context.Background()
		repo, commit := initGitRepo(t)
		out := t.TempDir()

		pkg, err := ap.Process("", pm, &core.Action{
			Name: "git",
			Spec: &core.Action_Git{Git: &core.ActionGitFetch{
				Url:    repo,
				Commit: commit,
			}},
		})
		assert.Loosely(t, err, should.BeNil)

		runWithDrv(t, ctx, pkg.Derivation, out)

		// Verify the repo was cloned and checked out
		fetched, err := git.PlainOpen(out)
		assert.Loosely(t, err, should.BeNil)
		head, err := fetched.Head()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, head.Hash().String(), should.Equal(commit))
	})
}

// go-git uses git binary for file repo. Override the behavior to ensure tests
// are hermetic.
type testRepoLoader struct{}

func (l *testRepoLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	path := ep.Path
	if ep.Host != "" {
		path = ep.Host + ":" + path
	}
	r, err := git.PlainOpen(filepath.FromSlash(path))
	if err != nil {
		return nil, err
	}
	return r.Storer, nil
}

func init() {
	client.InstallProtocol("test", server.NewClient(&testRepoLoader{}))
}

func initGitRepo(t *ftt.Test) (string, string) {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	assert.Loosely(t, err, should.BeNil)

	wt, err := repo.Worktree()
	assert.Loosely(t, err, should.BeNil)

	f, err := os.Create(filepath.Join(dir, "file"))
	assert.Loosely(t, err, should.BeNil)
	f.Close()
	_, err = wt.Add("file")
	assert.Loosely(t, err, should.BeNil)

	commit, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	assert.Loosely(t, err, should.BeNil)

	return "test://" + filepath.ToSlash(dir), commit.String()
}
