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
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg/internal/ui"
)

// Fetcher returns a new GitFetcher pinned to the given ref, commit, and
// pkgRoot in this repo.
//
// `ref` must be in the form `refs/*`, and must contain `commit`. If a ref is
// provided which does not contain `commit`, this returns ErrMissingObject.
//
// `commit` must be a full git commit id as hex.
//
// `pkgRoot` is the subdirectory within this commit that arguments to Read are
// relative to. If this is "", it means that the pkgRelPath's are relative to
// the root of the commit.
//
// `prefetch` MUST be provided, it will be called once for each entry in the git
// tree corresponding to `commit` which is contained within `pkgRoot`. If this
// returns `true`, then the indicated object will be added to a list of prefetch
// objects and fetched before this function returns. If this returns `false` for
// a TreeKind entry, traversing that entire subtree will be skipped.
//
// The returned GitFetcher will ONLY be allowed to read data prefetched by the
// `prefetch` function.
//
// Returns ErrMissingObject if `commit` or `commit:pkgRoot` are unknown.
func (r *RepoCache) Fetcher(ctx context.Context, ref, commit, pkgRoot string, prefetch func(kind ObjectKind, pkgRelPath string) bool) (*GitFetcher, error) {
	ctx = r.prepDebugContext(ctx)

	if r.repoRoot == "" {
		return nil, errors.New("gitsource.RepoCache must be constructed with gitsource.New")
	}
	if prefetch == nil {
		return nil, errors.New("gitsource.RepoCache.Fetcher: prefetch function is required (was nil)")
	}

	// test if $commit *and its trees* is in our repo
	_, err := r.batchProc.catFileTree(ctx, commit, "")
	if err != nil {
		if !errors.Is(err, ErrMissingObject) {
			return nil, err
		}
		// Fetch it
		ui.ActivityProgress(ctx, "fetching the commit")
		output, err := r.gitCombinedOutput(ctx, "fetch", "--depth", "1", "origin", commit)
		if err != nil {
			// If the remote doesn't understand what we want to fetch, return
			// ErrMissingObject immediately.
			if bytes.Contains(output, []byte("not our ref")) {
				err = ErrMissingObject
			}
			return nil, err
		}
	}

	if pkgRoot != "" {
		if cleaned := path.Clean(pkgRoot); cleaned != pkgRoot {
			return nil, fmt.Errorf("pkgRoot is not clean: %q (cleaned: %q)", pkgRoot, cleaned)
		}
	}

	// pkgRoot does not end with a "/" unless it is literally "/"
	// pkgRoot of "." is a bad way to spell ""
	if pkgRoot == "." || pkgRoot == "/" {
		pkgRoot = ""
	}

	ret := &GitFetcher{
		r:       r,
		ref:     ref,
		commit:  commit,
		pkgRoot: pkgRoot,
	}
	ret.allowedBlobPaths = stringset.New(100)
	var hashes []string
	trimOffset := len(pkgRoot)
	if trimOffset > 1 {
		// account for slash
		trimOffset++
	}

	ui.ActivityProgress(ctx, "finding missing objects")
	err = r.batchProc.catFileTreeWalk(ctx, commit, pkgRoot, nil, func(repoRelPath string, dirContent tree) (map[int]struct{}, error) {
		var skips map[int]struct{}

		for i, entry := range dirContent {
			entryPath := path.Join(repoRelPath, entry.name)
			pkgRelPath := entryPath[trimOffset:]
			ok := prefetch(entry.kind, pkgRelPath)
			switch entry.kind {
			case BlobKind:
				if ok {
					if r.debugLogs {
						logging.Debugf(ctx, "prefetching %s - %s", entryPath, entry.hash)
					}
					ret.allowedBlobPaths.Add(pkgRelPath)
					hashes = append(hashes, entry.hash)
				}
			case TreeKind:
				if !ok {
					if skips == nil {
						skips = make(map[int]struct{})
					}
					skips[i] = struct{}{}
				}
			}
		}
		return skips, nil
	})
	if err != nil {
		return nil, err
	}

	if len(hashes) > 0 {
		if err = r.prefetchMultiple(ctx, hashes); err != nil {
			return nil, err
		}
	}

	return ret, nil
}

type GitFetcher struct {
	r                *RepoCache
	ref              string
	commit           string
	pkgRoot          string
	allowedBlobPaths stringset.Set
}
