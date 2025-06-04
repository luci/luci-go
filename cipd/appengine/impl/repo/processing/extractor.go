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
	"hash"
	"io"
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/common"
)

// Extractor can extract files from the package, writing them to the CAS.
type Extractor struct {
	// Reader is an already open package file.
	Reader *PackageReader

	// CAS is the destination CAS implementation.
	CAS cas.StorageServer

	// PrimaryHash is the hash algorithm to use to name the file in the CAS.
	PrimaryHash api.HashAlgo

	// AlternativeHashes is a list of hashes to calculate in addition to
	// the PrimaryHash.
	AlternativeHashes []api.HashAlgo

	// Uploader returns io.Writer that uploads to the given destination URL.
	//
	// If nil, will use a Google Storage uploader. Useful in tests.
	Uploader func(ctx context.Context, size int64, uploadURL string) io.Writer

	// BufferSize is size of the buffer for GS uploads (default is 2 Mb).
	BufferSize int
}

// ExtractionResult is a result of a successful file extraction.
type ExtractionResult struct {
	Path   string                     // the file path passed to Run
	Ref    *api.ObjectRef             // reference to the extracted file in the CAS
	Size   int64                      // the size of the file in bytes
	Hashes map[api.HashAlgo]hash.Hash // all calculated hashes
}

// Run extracts a single file from the package.
func (ex *Extractor) Run(ctx context.Context, path string) (*ExtractionResult, error) {
	// Collect a map with all output hashes.
	hashes := make(map[api.HashAlgo]hash.Hash, len(ex.AlternativeHashes)+1)
	for _, algo := range ex.AlternativeHashes {
		hashes[algo] = common.MustNewHash(algo)
	}
	if hashes[ex.PrimaryHash] == nil {
		hashes[ex.PrimaryHash] = common.MustNewHash(ex.PrimaryHash)
	}

	// Start reading the file.
	reader, size, err := ex.Reader.Open(path)
	if err != nil {
		return nil, errors.Fmt("failed to open the file for reading: %w", err)
	}
	defer reader.Close() // we don't care about errors here

	// Start writing the result to CAS.
	op, err := ex.CAS.BeginUpload(ctx, &api.BeginUploadRequest{
		HashAlgo: ex.PrimaryHash,
	})
	if err != nil {
		return nil, transient.Tag.Apply(errors.

			// Grab an io.Writer that uploads to Google Storage.
			Fmt("failed to open a CAS upload: %w", err))
	}

	factory := ex.Uploader
	if factory == nil {
		factory = gsUploader
	}
	uploader := factory(ctx, size, op.UploadUrl)

	// Copy in 2 Mb chunks by default.
	bufferSize := ex.BufferSize
	if bufferSize == 0 {
		bufferSize = 2 * 1024 * 1024
	}

	// Copy, calculating digests on the fly.
	//
	// We use fullReader to make sure we write full 2 Mb chunks to GS. Otherwise
	// 'reader' uses 32 Kb buffers and they are flushed as 32 Kb buffers to Google
	// Storage too (which doesn't work). Remember, in Go an io.Reader can choose
	// to read less than asked and zip readers use 32 Kb buffers. CopyBuffer just
	// sends them to the writer right away.
	//
	// Note that reads from Google Storage are already properly buffered by
	// PackageReader implementation, so it's OK if the zip reader reads small
	// chunks from the underlying file reader. We basically read 512 Kb buffer
	// from GS, then unzip it in memory via small 32 Kb chunks into 2 Mb output
	// buffer, and then flush it to GS.
	writeTo := make([]io.Writer, 0, 1+len(hashes))
	writeTo = append(writeTo, uploader)
	for _, hash := range hashes {
		writeTo = append(writeTo, hash)
	}
	copied, err := io.CopyBuffer(
		io.MultiWriter(writeTo...),
		fullReader{reader},
		make([]byte, bufferSize))
	if err == nil && copied != size {
		err = errors.Fmt("unexpected file size: expecting %d bytes, read %d bytes", size, copied)
	}

	// If asked to rewind to a faraway offset (should be rare), just restart the
	// whole process from scratch by returning a transient error.
	if _, ok := err.(*gs.RestartUploadError); ok {
		err = transient.Tag.Apply(errors.Fmt("asked to restart the upload from faraway offset: %w", err))
	}

	if err != nil {
		// Best effort cleanup of the upload session. It's not a big deal if this
		// fails and the upload stays as garbage.
		_, cancelErr := ex.CAS.CancelUpload(ctx, &api.CancelUploadRequest{
			UploadOperationId: op.OperationId,
		})
		if cancelErr != nil {
			logging.Errorf(ctx, "Failed to cancel the upload: %s", cancelErr)
		}
		return nil, err
	}

	// Skip the hash calculation in CAS by enforcing the hash, we've just
	// calculated it.
	extractedRef := &api.ObjectRef{
		HashAlgo:  ex.PrimaryHash,
		HexDigest: common.HexDigest(hashes[ex.PrimaryHash]),
	}
	op, err = ex.CAS.FinishUpload(ctx, &api.FinishUploadRequest{
		UploadOperationId: op.OperationId,
		ForceHash:         extractedRef,
	})

	// CAS should publish the object right away.
	switch {
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to finalize the CAS upload: %w", err))
	case op.Status != api.UploadStatus_PUBLISHED:
		return nil, errors.Fmt("unexpected upload status from CAS %s: %s", op.Status, op.ErrorMessage)
	}

	// Success!
	return &ExtractionResult{
		Path:   path,
		Ref:    extractedRef,
		Size:   size,
		Hashes: hashes,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////

func gsUploader(ctx context.Context, size int64, uploadURL string) io.Writer {
	// Authentication is handled through the tokens in the upload session URL.
	tr, err := auth.GetRPCTransport(ctx, auth.NoAuth)
	if err != nil {
		panic(errors.Fmt("failed to get the RPC transport: %w", err))
	}
	return &gs.Uploader{
		Context:   ctx,
		Client:    &http.Client{Transport: tr},
		UploadURL: uploadURL,
		FileSize:  size,
	}
}

// fullReader is io.Reader that fills the buffer completely using the data from
// the underlying reader.
type fullReader struct {
	r io.ReadCloser
}

func (r fullReader) Read(buf []byte) (n int, err error) {
	n, err = io.ReadFull(r.r, buf)
	if err == io.ErrUnexpectedEOF {
		err = nil // this is fine, we are just reading the last chunk
	}
	return
}
