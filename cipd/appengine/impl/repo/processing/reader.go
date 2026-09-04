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
	"encoding/json"
	"io"
	"math"

	"github.com/klauspost/compress/zip"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/cipd/client/cipd/pkg"
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

// maximumManifestJSONSize limits the size of the manifest JSON we are willing
// to extract from an uploaded package instance.
//
// As of 2026Q3, the only data in the manifest is the manifest version, the
// package name, and the version file path.
//
// There are fields for file->hash associations, but these are only populated
// by the CIPD client on disk during extraction.
const maximumManifestJSONSize = 2 * 1024

// Opens, parses and returns the Manifest in this package instance.
//
// Returns InvalidArgument for non-transient errors.
func (p *PackageReader) Manifest() (pkg.Manifest, error) {
	tagError := func(err error) error {
		if transient.Tag.In(err) {
			return err
		}
		return status.Errorf(codes.InvalidArgument, "%s", err)
	}

	raw, decompSize, err := p.Open(pkg.ManifestName)
	if err != nil {
		return pkg.Manifest{}, tagError(errors.Fmt("opening manifest file: %w", err))
	}
	defer raw.Close()
	if decompSize > maximumManifestJSONSize {
		return pkg.Manifest{}, tagError(errors.Fmt(
			"manifest file is too large: %d > %d", decompSize, maximumManifestJSONSize))
	}
	rawDat, err := io.ReadAll(raw)
	if err != nil {
		return pkg.Manifest{}, tagError(errors.Fmt("reading manifest file: %w", err))
	}

	ret := pkg.Manifest{}
	if err = json.Unmarshal(rawDat, &ret); err != nil {
		return pkg.Manifest{}, tagError(errors.Fmt("decoding manifest file: %w", err))
	}
	return ret, nil
}
