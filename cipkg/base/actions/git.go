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
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"go.chromium.org/luci/cipkg/core"
)

// ActionGitFetchTransformer is the default transformer for
// core.ActionGitFetch.
func ActionGitFetchTransformer(a *core.ActionGitFetch, deps []Package) (*core.Derivation, error) {
	return ReexecDerivation(a, false)
}

// ActionGitFetchExecutor is the default executor for core.ActionGitFetch.
func ActionGitFetchExecutor(ctx context.Context, a *core.ActionGitFetch, out string) error {
	commit := plumbing.NewHash(a.Commit)
	if commit.IsZero() {
		return fmt.Errorf("invalid git commit: %s", a.Commit)
	}

	repo, err := git.PlainCloneContext(ctx, out, false, &git.CloneOptions{
		URL:        a.Url,
		NoCheckout: true,
	})
	if err != nil {
		return fmt.Errorf("failed to clone git repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	if err = wt.Checkout(&git.CheckoutOptions{Hash: commit}); err != nil {
		return fmt.Errorf("failed to checkout commit: %w", err)
	}

	return nil
}
