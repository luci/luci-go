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

package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/testutils"
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
		repo, commit := testutils.InitGitRepo(t)
		out := t.TempDir()

		t.Run("Test clone and checkout commit", func(t *ftt.Test) {
			a := &core.ActionGitFetch{
				Url:    repo,
				Commit: commit,
				Export: true,
			}

			err := ActionGitFetchExecutor(ctx, a, out)
			assert.Loosely(t, err, should.BeNil)

			// Verify the repo was cloned and checked out
			_, err = os.Stat(filepath.Join(out, "file"))
			assert.Loosely(t, err, should.BeNil)

			// Verify .git does not exist in out
			_, err = os.Stat(filepath.Join(out, ".git"))
			assert.Loosely(t, os.IsNotExist(err), should.BeTrue)
		})

		t.Run("Test not export", func(t *ftt.Test) {
			a := &core.ActionGitFetch{
				Url:    repo,
				Commit: commit,
				Export: false,
			}
			out2 := t.TempDir()

			err := ActionGitFetchExecutor(ctx, a, out2)
			assert.Loosely(t, err, should.BeNil)

			// Verify the repo was cloned and checked out
			_, err = os.Stat(filepath.Join(out2, "file"))
			assert.Loosely(t, err, should.BeNil)

			// Verify .git does exist in out
			_, err = os.Stat(filepath.Join(out2, ".git"))
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Test clone and checkout commit with submodule", func(t *ftt.Test) {
			subRepo, subCommit := testutils.InitGitRepoWithSubmodule(t)
			out3 := t.TempDir()

			a := &core.ActionGitFetch{
				Url:       subRepo,
				Commit:    subCommit,
				Export:    true,
				Recursive: true,
			}

			err := ActionGitFetchExecutor(ctx, a, out3)
			assert.Loosely(t, err, should.BeNil)

			// Verify the repo was cloned and checked out
			_, err = os.Stat(filepath.Join(out3, "file"))
			assert.Loosely(t, err, should.BeNil)

			// Verify submodule was cloned and checked out
			_, err = os.Stat(filepath.Join(out3, "sub", "subfile"))
			assert.Loosely(t, err, should.BeNil)

			// Verify .git does not exist in out
			_, err = os.Stat(filepath.Join(out3, ".git"))
			assert.Loosely(t, os.IsNotExist(err), should.BeTrue)

			// Verify submodule .git does not exist
			_, err = os.Stat(filepath.Join(out3, "sub", ".git"))
			assert.Loosely(t, os.IsNotExist(err), should.BeTrue)
		})
	})
}

func TestReexecGit(t *testing.T) {
	ftt.Run("Test re-execute action processor for git", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")
		ctx := context.Background()
		repo, commit := testutils.InitGitRepo(t)
		out := t.TempDir()

		pkg, err := ap.Process("", pm, &core.Action{
			Name: "git",
			Spec: &core.Action_Git{Git: &core.ActionGitFetch{
				Url:    repo,
				Commit: commit,
				Export: true,
			}},
		})
		assert.Loosely(t, err, should.BeNil)

		runWithDrv(t, ctx, pkg.Derivation, out)

		// Verify the repo was cloned and checked out
		_, err = os.Stat(filepath.Join(out, "file"))
		assert.Loosely(t, err, should.BeNil)

		// Verify .git does not exist in out
		_, err = os.Stat(filepath.Join(out, ".git"))
		assert.Loosely(t, os.IsNotExist(err), should.BeTrue)
	})
}
