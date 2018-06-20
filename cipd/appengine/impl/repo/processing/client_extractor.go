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
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/common"
)

// ClientExtractorProcID is identifier of ClientExtractor processor.
const ClientExtractorProcID = "cipd_client_binary:v1"

const clientPkgPrefix = "infra/tools/cipd/"

// GetClientPackage returns the name of the client package for CIPD client for
// the given platform.
//
// Returns an error if the platform name is invalid.
func GetClientPackage(platform string) (string, error) {
	pkg := clientPkgPrefix + platform
	if err := common.ValidatePackageName(pkg); err != nil {
		return "", err
	}
	return pkg, nil
}

// IsClientPackage returns true if the given package stores a CIPD client.
func IsClientPackage(pkg string) bool {
	return strings.HasPrefix(pkg, clientPkgPrefix)
}

// GetClientBinaryName returns name of CIPD binary inside the package.
//
// Either 'cipd' or 'cipd.exe'.
func GetClientBinaryName(pkg string) string {
	if strings.HasPrefix(pkg, clientPkgPrefix+"windows-") {
		return "cipd.exe"
	}
	return "cipd"
}

// ClientExtractorResult is stored in JSON form as a result of ClientExtractor
// execution.
//
// Compatible with Python version of the backend.
//
// If format of this struct changes in a non backward compatible way, the
// version number in ClientExtractorProcID should change too.
type ClientExtractorResult struct {
	ClientBinary struct {
		Size       int64  `json:"size"`
		HashAlgo   string `json:"hash_algo"`
		HashDigest string `json:"hash_digest"`
	} `json:"client_binary"`
}

// ToObjectRef returns a reference to the extracted client binary in CAS.
//
// The returned ObjectRef is validated to be syntactically correct already.
func (r *ClientExtractorResult) ToObjectRef() (*api.ObjectRef, error) {
	if r.ClientBinary.HashAlgo != "SHA1" {
		return nil, fmt.Errorf("expecting SHA1 hash, got %q", r.ClientBinary.HashAlgo)
	}
	ref := &api.ObjectRef{
		HashAlgo:  api.HashAlgo_SHA1,
		HexDigest: r.ClientBinary.HashDigest,
	}
	if err := common.ValidateObjectRef(ref); err != nil {
		return nil, err
	}
	return ref, nil
}

// SHA1 returns client's SHA1 hash or "" if not known.
//
// This methods exists in anticipation that ObjectRef will become SHA256-based
// soon, but we still need to return SHA1 to older clients and bootstrap scripts
// that don't understand SHA256.
func (r *ClientExtractorResult) SHA1() string {
	if strings.ToLower(r.ClientBinary.HashAlgo) == "sha1" {
		return r.ClientBinary.HashDigest
	}
	return ""
}

// ClientExtractor is a processor that extracts CIPD client binary from CIPD
// client packages (infra/tools/cipd/...) and stores it in the CAS, so it can be
// fetched directly.
//
// This is needed to support CIPD client bootstrap using e.g. 'curl'.
type ClientExtractor struct {
	CAS cas.StorageServer

	// uploader returns an io.Writer to push all extracted data to.
	//
	// Default is gsUploader, but can be mocked in tests.
	uploader func(ctx context.Context, size int64, uploadURL string) io.Writer

	// bufferSize is size of the buffer for GS uploads (default is 2 Mb).
	bufferSize int
}

// ID is part of Processor interface.
func (e *ClientExtractor) ID() string {
	return ClientExtractorProcID
}

// Applicable is part of Processor interface.
func (e *ClientExtractor) Applicable(inst *model.Instance) bool {
	return IsClientPackage(inst.Package.StringID())
}

// Run is part of Processor interface.
func (e *ClientExtractor) Run(ctx context.Context, inst *model.Instance, pkg *PackageReader) (res Result, err error) {
	// Put fatal errors into 'res' and return transient ones as is.
	defer func() {
		if err != nil && !transient.Tag.In(err) {
			res.Err = err
			err = nil
		}
	}()

	// Start extracting the file.
	reader, size, err := pkg.Open(GetClientBinaryName(inst.Package.StringID()))
	if err != nil {
		err = errors.Annotate(err, "failed to open the file for reading").Err()
		return
	}
	defer reader.Close() // we don't care about errors here

	// Start writing the result to CAS.
	op, err := e.CAS.BeginUpload(ctx, &api.BeginUploadRequest{
		HashAlgo: api.HashAlgo_SHA1,
	})
	if err != nil {
		err = errors.Annotate(err, "failed to open a CAS upload").Tag(transient.Tag).Err()
		return
	}

	// Grab an io.Writer that uploads to Google Storage.
	factory := e.uploader
	if factory == nil {
		factory = gsUploader
	}
	uploader := factory(ctx, size, op.UploadUrl)

	// Copy in 2 Mb chunks by default.
	bufferSize := e.bufferSize
	if bufferSize == 0 {
		bufferSize = 2 * 1024 * 1024
	}

	// Copy calculating SHA1 on the fly.
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
	hash, _ := common.NewHash(api.HashAlgo_SHA1)
	copied, err := io.CopyBuffer(
		io.MultiWriter(uploader, hash),
		fullReader{reader},
		make([]byte, bufferSize))
	if err == nil && copied != size {
		err = fmt.Errorf("unexpected file size - expecting %d bytes, read %d bytes", size, copied)
	}

	// If asked to rewind to a faraway offset (should be rare), just restart the
	// whole process from scratch by returning a transient error.
	if _, ok := err.(*gs.RestartUploadError); ok {
		err = errors.Annotate(err, "asked to restart the upload from faraway offset").Tag(transient.Tag).Err()
	}

	if err != nil {
		// Best effort cleanup of the upload session. It's not a big deal if this
		// fails and the upload stays as garbage.
		_, cancelErr := e.CAS.CancelUpload(ctx, &api.CancelUploadRequest{
			UploadOperationId: op.OperationId,
		})
		if cancelErr != nil {
			logging.WithError(cancelErr).Errorf(ctx, "Failed to cancel the upload")
		}
		return
	}

	// Skip the hash calculation in CAS by enforcing the hash, we've just
	// calculated it.
	digest := hex.EncodeToString(hash.Sum(nil))
	op, err = e.CAS.FinishUpload(ctx, &api.FinishUploadRequest{
		UploadOperationId: op.OperationId,
		ForceHash: &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: digest,
		},
	})

	// CAS should publish the object right away.
	switch {
	case err != nil:
		err = errors.Annotate(err, "failed to finalize the CAS upload").Tag(transient.Tag).Err()
		return
	case op.Status != api.UploadStatus_PUBLISHED:
		err = errors.Reason("unexpected upload status from CAS %s: %s", op.Status, op.ErrorMessage).Err()
		return
	}

	logging.Infof(ctx, "Uploaded CIPD client binary %s:%s (%d bytes)", inst.Package.StringID(), digest, size)

	r := ClientExtractorResult{}
	r.ClientBinary.Size = size
	r.ClientBinary.HashAlgo = "SHA1"
	r.ClientBinary.HashDigest = digest

	res.Result = r
	return
}

// GetClientExtractorResult returns results of client extractor processor.
//
// They contain a reference to the unpacked CIPD binary object in the Google
// Storage.
//
// Returns:
//   (result, nil) on success.
//   (nil, datastore.ErrNoSuchEntity) if results are not available.
//   (nil, transient-tagged error) on retrieval errors.
//   (nil, non-transient-tagged error) if the client extractor failed.
func GetClientExtractorResult(c context.Context, inst *api.Instance) (*ClientExtractorResult, error) {
	r := &model.ProcessingResult{
		ProcID:   ClientExtractorProcID,
		Instance: datastore.KeyForObj(c, (&model.Instance{}).FromProto(c, inst)),
	}
	switch err := datastore.Get(c, r); {
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	case err != nil:
		return nil, transient.Tag.Apply(err)
	case !r.Success:
		return nil, errors.Reason("client extraction failed - %s", r.Error).Err()
	}
	out := &ClientExtractorResult{}
	if err := r.ReadResult(out); err != nil {
		return nil, errors.Annotate(err, "failed to parse the client extractor status").Err()
	}
	return out, nil
}

////////////////////////////////////////////////////////////////////////////////

func gsUploader(ctx context.Context, size int64, uploadURL string) io.Writer {
	// Authentication is handled through the tokens in the upload session URL.
	tr, err := auth.GetRPCTransport(ctx, auth.NoAuth)
	if err != nil {
		panic(errors.Annotate(err, "failed to get the RPC transport").Err())
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
