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
	"crypto/sha1"
	"crypto/sha256"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"

	"go.chromium.org/luci/cipkg/core"
)

// FetchGit is a generator for fetching git repository.
type FetchGit struct {
	Name     string
	Metadata *core.Action_Metadata

	URL string
	Ref string
}

func (g *FetchGit) Generate(ctx context.Context, plats Platforms) (*core.Action, error) {
	commit := parseCommit(g.Ref)
	if commit.IsZero() {
		var err error
		if commit, err = resolveRef(ctx, g.URL, g.Ref); err != nil {
			return nil, fmt.Errorf("failed to resolve git ref %s: %w", g.Ref, err)
		}
	}

	return &core.Action{
		Name:     g.Name,
		Metadata: g.Metadata,
		Spec: &core.Action_Git{
			Git: &core.ActionGitFetch{
				Url:    g.URL,
				Commit: commit.String(),
			},
		},
	}, nil
}

func parseCommit(s string) plumbing.Hash {
	switch len(s) {
	case sha1.Size * 2:
	case sha256.Size * 2:
	default:
		return plumbing.Hash{}
	}
	return plumbing.NewHash(s)
}

func resolveRef(ctx context.Context, url, ref string) (plumbing.Hash, error) {
	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{url},
	})

	refs, err := remote.ListContext(ctx, &git.ListOptions{})
	if err != nil {
		return plumbing.Hash{}, err
	}

	for _, r := range refs {
		if r.Name().Short() == ref || r.Name().String() == ref {
			return r.Hash(), nil
		}
	}

	return plumbing.Hash{}, fmt.Errorf("ref %s not found in %s", ref, url)
}
