// Copyright 2018 The LUCI Authors.
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

package processing

import (
	"io"
	"math"

	"github.com/klauspost/compress/zip"

	"go.chromium.org/luci/common/errors"
)

// TODO(vadimsh): Share code with the client.

// PackageReader knows how to extract files from CIPD packages.
//
// CIPD packages are actually zip archives, but we don't want to expose it
// everywhere.
type PackageReader struct {
	zr *zip.Reader
}

// NewPackageReader opens the package by reading its directory.
func NewPackageReader(r io.ReaderAt, size int64) (*PackageReader, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		// Note: we rely here (and in other places where we return errors) on
		// zip.Reader NOT wrapping errors from 'r', so they inherit transient tags
		// in case of transient Google Storage errors.
		return nil, err
	}
	return &PackageReader{zr}, nil
}

// Files returns names of files inside the package.
func (p *PackageReader) Files() []string {
	files := make([]string, len(p.zr.File))
	for i, f := range p.zr.File {
		files[i] = f.Name
	}
	return files
}

// Open opens some file inside the package for reading.
//
// Returns the ReadCloser and the uncompressed file size.
func (p *PackageReader) Open(path string) (io.ReadCloser, int64, error) {
	for _, f := range p.zr.File {
		if f.Name == path {
			if f.UncompressedSize64 > math.MaxInt64 {
				return nil, 0, errors.Fmt("the file %q is unbelievably huge (%d bytes)", path, f.UncompressedSize64)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, 0, err
			}
			return rc, int64(f.UncompressedSize64), nil
		}
	}
	return nil, 0, errors.Fmt("no file %q inside the package", path)
}
