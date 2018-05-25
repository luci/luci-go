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
	"archive/zip"
	"io"

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

// Open opens some file inside the package for reading.
func (p *PackageReader) Open(path string) (io.ReadCloser, error) {
	for _, f := range p.zr.File {
		if f.Name == path {
			return f.Open()
		}
	}
	return nil, errors.Reason("no file %q inside the package", path).Err()
}
