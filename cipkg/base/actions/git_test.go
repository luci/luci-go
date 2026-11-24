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

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/testutils"
)

func TestProcessGit(t *testing.T) {
	ftt.Run("Test action processor for git", t, func(t *ftt.Test) {
		ctx := context.Background()
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		gitSpec := &core.ActionGitFetch{
			Url:    "https://host.not.exist/repo",
			Commit: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		}

		pkg, err := ap.Process(ctx, "", pm, &core.Action{
			Name: "git",
			Spec: &core.Action_Git{Git: gitSpec},
		})
		assert.Loosely(t, err, should.BeNil)

		checkReexecArg(t, pkg.Derivation.Args, gitSpec)

		t.Run("forwards proxy and TLS env without leaking unrelated host env or changing DerivationID", func(t *ftt.Test) {
			envCtx := environ.New([]string{
				"https_proxy=http://127.0.0.1:8443",
				"NO_PROXY=localhost",
				"SSL_CERT_DIR=/etc/ssl/certs",
				"UNRELATED_HOST_ENV=should-not-leak",
			}).SetInCtx(ctx)

			proxiedPkg, err := ap.Process(envCtx, "", pm, &core.Action{
				Name: "git",
				Spec: &core.Action_Git{Git: gitSpec},
			})
			assert.NoErr(t, err)
			assert.That(t, proxiedPkg.DerivationID, should.Equal(pkg.DerivationID))

			drvEnv := environ.New(proxiedPkg.Derivation.Env)
			assert.That(t, drvEnv.Get("https_proxy"), should.Equal("http://127.0.0.1:8443"))
			assert.That(t, drvEnv.Get("NO_PROXY"), should.Equal("localhost"))
			assert.That(t, drvEnv.Get("SSL_CERT_DIR"), should.Equal("/etc/ssl/certs"))
			assert.That(t, drvEnv.Get("UNRELATED_HOST_ENV"), should.Equal(""))
			assert.That(t, drvEnv.Get(envCipkgExec), should.Equal("1"))
		})
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
		out := t.TempDir()

		t.Run("direct", func(t *ftt.Test) {
			repo, commit := testutils.InitGitRepo(t)
			pkg, err := ap.Process(ctx, "", pm, &core.Action{
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

		t.Run("via http_proxy with submodule", func(t *ftt.Test) {
			proxyURL, repoURL, commit, proxyHits := testutils.InitGitHTTPProxy(t)

			clearProxyEnv(t)
			t.Setenv("http_proxy", proxyURL)

			pkg, err := ap.Process(ctx, "", pm, &core.Action{
				Name: "git",
				Spec: &core.Action_Git{Git: &core.ActionGitFetch{
					Url:       repoURL,
					Commit:    commit,
					Export:    true,
					Recursive: true,
				}},
			})
			assert.NoErr(t, err)

			assert.NoErr(t, execDrv(t, ctx, pkg.Derivation, out))
			assert.That(t, proxyHits.Load() > 0, should.BeTrue)

			b, err := os.ReadFile(filepath.Join(out, "file"))
			assert.NoErr(t, err)
			assert.That(t, string(b), should.Equal("data"))

			subBytes, err := os.ReadFile(filepath.Join(out, "sub", "subfile"))
			assert.NoErr(t, err)
			assert.That(t, string(subBytes), should.Equal("subdata"))
		})
	})
}
