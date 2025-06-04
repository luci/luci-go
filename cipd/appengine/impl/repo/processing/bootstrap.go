// Copyright 2021 The LUCI Authors.
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
	"context"
	"io"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/bootstrap"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/cipd/appengine/impl/model"
)

// BootstrapPackageExtractorProcID is identifier of BootstrapPackageExtractor.
const BootstrapPackageExtractorProcID = "bootstrap_extractor:v1"

// BootstrapPackageExtractor is a processor that extracts files from bootstrap
// packages (per bootstrap.cfg service config).
type BootstrapPackageExtractor struct {
	CAS cas.StorageServer

	// For mocking in tests.
	uploader func(ctx context.Context, size int64, uploadURL string) io.Writer
}

// BootstrapExtractorResult is stored as JSON in model.ProcessingResult.
type BootstrapExtractorResult struct {
	File       string // the name of the extracted file
	HashAlgo   string // the hash algorithm used to calculate its hash for CAS
	HashDigest string // its hex digest in the CAS
	Size       int64  // the size of the extracted file
}

// ID is a part of the Processor interface.
func (bs *BootstrapPackageExtractor) ID() string {
	return BootstrapPackageExtractorProcID
}

// Applicable is a part of the Processor interface.
func (bs *BootstrapPackageExtractor) Applicable(ctx context.Context, inst *model.Instance) (bool, error) {
	cfg, err := bootstrap.BootstrapConfig(ctx, inst.Package.StringID())
	return cfg != nil, err
}

// Run is a part of the Processor interface.
func (bs *BootstrapPackageExtractor) Run(ctx context.Context, inst *model.Instance, pkg *PackageReader) (res Result, err error) {
	// Put fatal errors into 'res' and return transient ones as is.
	defer func() {
		if err != nil && !transient.Tag.In(err) {
			res.Err = err
			err = nil
		}
	}()

	// Bootstrap packages are expected to contain only one top-level file (which
	// we assume is the executable used for the bootstrap). Note that all packages
	// contain ".cipdpkg" directory with some CIPD metadata, which we skip.
	//
	// For windows, a bat shim might be produced. If this is the case, we ignore
	// it here since it is not the executable.
	executable := ""
	for _, f := range pkg.Files() {
		if strings.HasPrefix(f, ".cipdpkg/") {
			continue
		}
		if strings.HasSuffix(f, ".bat") {
			continue
		}
		if executable != "" {
			err = errors.New("the package is marked as a bootstrap package, but it contains multiple files")
			return
		}
		executable = f
	}
	switch {
	case executable == "":
		err = errors.New("the package is marked as a bootstrap package, but it contains no files")
		return
	case strings.Contains(executable, "/"):
		err = errors.New("the package is marked as a bootstrap package, but its content is not at the package root")
		return
	}

	// Execute the extraction.
	result, err := (&Extractor{
		Reader:      pkg,
		CAS:         bs.CAS,
		PrimaryHash: api.HashAlgo_SHA256,
		Uploader:    bs.uploader,
	}).Run(ctx, executable)
	if err != nil {
		return
	}

	// Store the results in the appropriate format.
	res.Result = BootstrapExtractorResult{
		File:       executable,
		HashAlgo:   result.Ref.HashAlgo.String(),
		HashDigest: result.Ref.HexDigest,
		Size:       result.Size,
	}

	logging.Infof(ctx, "Extracted the bootstrap executable %q from %s: %s %s (%d bytes)",
		executable, inst.Package.StringID(), result.Ref.HashAlgo, result.Ref.HexDigest, result.Size)

	return
}

// GetBootstrapExtractorResult returns results of BootstrapPackageExtractor.
//
// Returns:
//
//	(result, nil) on success.
//	(nil, datastore.ErrNoSuchEntity) if results are not available.
//	(nil, transient-tagged error) on retrieval errors.
//	(nil, non-transient-tagged error) if the extractor failed.
func GetBootstrapExtractorResult(ctx context.Context, inst *model.Instance) (*BootstrapExtractorResult, error) {
	r := &model.ProcessingResult{
		ProcID:   BootstrapPackageExtractorProcID,
		Instance: datastore.KeyForObj(ctx, inst),
	}
	switch err := datastore.Get(ctx, r); {
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	case err != nil:
		return nil, transient.Tag.Apply(err)
	case !r.Success:
		return nil, errors.Fmt("bootstrap extraction failed: %s", r.Error)
	}
	out := &BootstrapExtractorResult{}
	if err := r.ReadResult(out); err != nil {
		return nil, errors.Fmt("failed to parse the extractor status: %w", err)
	}
	return out, nil
}
