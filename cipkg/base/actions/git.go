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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/client"
	"github.com/go-git/go-git/v6/plumbing/transport"

	"go.chromium.org/luci/cipkg/core"
)

// ActionGitFetchTransformer is the default transformer for
// core.ActionGitFetch.
func ActionGitFetchTransformer(ctx context.Context, a *core.ActionGitFetch, deps []Package) (*core.Derivation, error) {
	return ReexecDerivation(a, deps, passthroughEnv(ctx, httpEnvVars...))
}

// ActionGitFetchExecutor is the default executor for core.ActionGitFetch.
func ActionGitFetchExecutor(ctx context.Context, a *core.ActionGitFetch, out string) error {
	commit := plumbing.NewHash(a.Commit)
	if commit.IsZero() {
		return fmt.Errorf("invalid git commit: %s", a.Commit)
	}

	if err := gitFetchRepo(ctx, a, commit, out); err != nil {
		return err
	}

	// Clean up any .git files or directories that may have been created
	// in the worktree (e.g., inside submodule directories).
	if a.Export {
		if err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Name() == ".git" {
				if err := os.RemoveAll(path); err != nil {
					return err
				}
				if d.IsDir() {
					return filepath.SkipDir
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed to remove .git metadata: %w", err)
		}
	}

	return nil
}

func gitFetchRepo(ctx context.Context, a *core.ActionGitFetch, commit plumbing.Hash, out string) (err error) {
	clientOpts := []client.Option{client.WithProxyEnvironment()}

	repo, err := git.PlainInit(out, false)
	if err != nil {
		return fmt.Errorf("failed to init git repo: %w", err)
	}
	defer func() { err = errors.Join(err, repo.Close()) }()

	remote, err := repo.CreateRemote(&config.RemoteConfig{
		Name:  git.DefaultRemoteName,
		URLs:  []string{a.Url},
		Fetch: []config.RefSpec{config.RefSpec(fmt.Sprintf(config.DefaultFetchRefSpec, git.DefaultRemoteName))},
	})
	if err != nil {
		return fmt.Errorf("failed to create git remote: %w", err)
	}

	// Fetch only the requested commit at depth 1 when supported by the server.
	// Fall back to a full fetch if the server does not support exact-SHA1 or
	// shallow fetches.
	if err = remote.FetchContext(ctx, &git.FetchOptions{
		RefSpecs:      []config.RefSpec{config.RefSpec("+" + commit.String() + ":refs/remotes/origin/HEAD")},
		Depth:         1,
		Tags:          plumbing.NoTags,
		ClientOptions: clientOpts,
	}); err != nil {
		if !errors.Is(err, git.ErrExactSHA1NotSupported) && !errors.Is(err, transport.ErrShallowNotSupported) {
			return fmt.Errorf("failed to fetch git repo: %w", err)
		}
		if err = remote.FetchContext(ctx, &git.FetchOptions{
			Tags:          plumbing.AllTags,
			ClientOptions: clientOpts,
		}); err != nil {
			return fmt.Errorf("failed to fetch git repo: %w", err)
		}
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	if err = wt.Checkout(&git.CheckoutOptions{Hash: commit}); err != nil {
		return fmt.Errorf("failed to checkout commit: %w", err)
	}

	// Checkout git submodules recursively.
	if a.Recursive {
		subs, err := wt.Submodules()
		if err != nil {
			return fmt.Errorf("failed to get submodules: %w", err)
		}

		if err := subs.UpdateContext(ctx, &git.SubmoduleUpdateOptions{
			Init:              true,
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
			Depth:             1,
			ClientOptions:     clientOpts,
		}); err != nil {
			return fmt.Errorf("failed to update submodules: %w", err)
		}
	}

	return nil
}
