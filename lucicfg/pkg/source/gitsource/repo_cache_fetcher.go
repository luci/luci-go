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
	"path"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg/internal/ui"
	"go.chromium.org/luci/lucicfg/pkg/source"
)

func (r *repoCache) Fetcher(ctx context.Context, ref, commit, pkgRoot string, prefetch func(kind source.ObjectKind, pkgRelPath string) bool) (source.Fetcher, error) {
	ctx = r.prepDebugContext(ctx)

	if r.repoRoot == "" {
		return nil, errors.New("gitsource.RepoCache must be constructed with gitsource.New")
	}
	if prefetch == nil {
		return nil, errors.New("gitsource.RepoCache.Fetcher: prefetch function is required (was nil)")
	}
	pkgRoot, err := source.NormalizePkgRoot(pkgRoot)
	if err != nil {
		return nil, err
	}

	// test if $commit *and its trees* is in our repo
	_, err = r.batchProc.catFileTree(ctx, commit, "")
	if err != nil {
		if !errors.Is(err, source.ErrMissingObject) {
			return nil, err
		}
		// Fetch it
		ui.ActivityProgress(ctx, "fetching the commit")
		output, err := r.gitCombinedOutput(ctx, "fetch", "--depth", "1", "origin", commit)
		if err != nil {
			// If the remote doesn't understand what we want to fetch, return
			// ErrMissingObject immediately.
			if bytes.Contains(output, []byte("not our ref")) {
				err = source.ErrMissingObject
			}
			return nil, err
		}
	}

	ret := &gitFetcher{
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
			case source.BlobKind:
				if ok {
					if r.debugLogs {
						logging.Debugf(ctx, "prefetching %s - %s", entryPath, entry.hash)
					}
					ret.allowedBlobPaths.Add(pkgRelPath)
					hashes = append(hashes, entry.hash)
				}
			case source.TreeKind:
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

type gitFetcher struct {
	r                *repoCache
	ref              string
	commit           string
	pkgRoot          string
	allowedBlobPaths stringset.Set
}
