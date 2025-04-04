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
	"io"
	"path"
)

var ErrMissingObject = errors.New("object is missing")
var ErrObjectNotPrefetched = errors.New("object was not prefeched")

// Read implements depsource.Fetcher, and returns a reader over the bytes of the
// blob indicated by pkgRelPath.
//
// Returns ErrObjectNotPrefetched if the object wasn't prefetched.
func (g *GitFetcher) Read(ctx context.Context, pkgRelPath string) (io.ReadCloser, error) {
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
	return io.NopCloser(bytes.NewReader(data)), nil
}
