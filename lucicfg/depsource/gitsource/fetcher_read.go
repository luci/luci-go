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
	"context"
	"errors"
	"fmt"
	"path"

	"go.chromium.org/luci/lucicfg/depsource"
)

var (
	ErrMissingCommit       = errors.New("commit is missing")
	ErrMissingObject       = errors.New("object is missing")
	ErrObjectNotPrefetched = errors.New("object was not prefeched")
)

var _ depsource.Fetcher = (*GitFetcher)(nil)

// Read returns the bytes of the blob indicated by pkgRelPath.
//
// Returns ErrObjectNotPrefetched if the object wasn't prefetched.
func (g *GitFetcher) Read(ctx context.Context, pkgRelPath string) ([]byte, error) {
	ctx = g.r.prepDebugContext(ctx)

	if cleaned := path.Clean(pkgRelPath); pkgRelPath != cleaned {
		return nil, fmt.Errorf("pkgRelPath is not clean: %q (cleaned=%q)", pkgRelPath, cleaned)
	}
	if !g.allowedBlobPaths.Has(pkgRelPath) {
		return nil, fmt.Errorf("GitFetcher.Read: %q: %w", pkgRelPath, ErrObjectNotPrefetched)
	}

	data, err := g.r.batchProc.catFileBlob(ctx, g.commit, path.Join(g.pkgRoot, pkgRelPath))
	if err != nil {
		return nil, err
	}
	return data, nil
}
