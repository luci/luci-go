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

package gitilessource

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"path"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/lucicfg/pkg/source"
)

type fetcher struct {
	zipFile          *zip.ReadCloser
	pkgRoot          string
	allowedBlobPaths stringset.Set
}

func (g *fetcher) Read(ctx context.Context, pkgRelPath string) ([]byte, error) {
	if cleaned := path.Clean(pkgRelPath); pkgRelPath != cleaned {
		return nil, errors.Fmt("GitilesFetcher.Read: pkgRelPath is not clean: %q (cleaned=%q)", pkgRelPath, cleaned)
	}
	if !g.allowedBlobPaths.Has(pkgRelPath) {
		return nil, errors.Fmt("GitilesFetcher.Read: %q: %w", pkgRelPath, source.ErrObjectNotPrefetched)
	}

	var seenLinks stringset.Set

	for {
		ofile, err := g.zipFile.Open(path.Join(g.pkgRoot, pkgRelPath))
		if err != nil {
			return nil, errors.Fmt("GitilesFetcher.Read: %q: %w", pkgRelPath, err)
		}
		st, err := ofile.Stat()
		if err != nil {
			return nil, errors.Fmt("GitilesFetcher.Read.Stat(): %q: %w", pkgRelPath, err)
		}
		mode := st.Mode()
		if mode.IsRegular() {
			return io.ReadAll(ofile)
		}

		if (mode & fs.ModeSymlink) != 0 {
			if seenLinks == nil {
				seenLinks = stringset.NewFromSlice(pkgRelPath)
			} else {
				seenLinks.Add(pkgRelPath)
			}

			target, err := io.ReadAll(ofile)
			if err != nil {
				return nil, errors.Fmt("GitilesFetcher.Read.ReadAll: %q: %w", pkgRelPath, err)
			}
			pkgRelPath = path.Join(path.Dir(pkgRelPath), string(target))

			if seenLinks.Has(pkgRelPath) {
				return nil, errors.Fmt("GitilesFetcher.Read: symlink cycle: %q", pkgRelPath)
			}
		} else {
			return nil, errors.Fmt("GitilesFetcher.Read.Stat: %q: unsupported file mode %q", pkgRelPath, mode)
		}
	}
}
