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
	"context"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"

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
		Size int64 `json:"size"`

		// Algo used to name the extracted file, matches the client package algo.
		HashAlgo   string `json:"hash_algo"`   // cas.HashAlgo enum serialized to string
		HashDigest string `json:"hash_digest"` // as hex string

		// AllHashDigests are hex digests of the extracted file calculated using all
		// algos known to the server at the time the file was uploaded.
		//
		// Keys are cas.HashAlgo enum values as strings ('SHA1', 'SHA256', ...).
		//
		// If empty (for old records), only supported algo is HashAlgo from above
		// (which for old records is always SHA1).
		AllHashDigests map[string]string `json:"all_hash_digests"`
	} `json:"client_binary"`
}

// ToObjectRef returns a reference to the extracted client binary in CAS.
//
// The returned ObjectRef is validated to be syntactically correct already.
func (r *ClientExtractorResult) ToObjectRef() (*api.ObjectRef, error) {
	algo := api.HashAlgo_value[r.ClientBinary.HashAlgo]
	if algo == 0 {
		// Note: this means OLD version of the server may not be able to serve
		// NEW ClientExtractorResult entries due to unknown hash algo. Many other
		// things will also break in this situation. If this is really happening,
		// all new entries can be manually removed from the datastore, to stop
		// confusing the old server version.
		return nil, fmt.Errorf("unrecognized hash algo %q", r.ClientBinary.HashAlgo)
	}
	ref := &api.ObjectRef{
		HashAlgo:  api.HashAlgo(algo),
		HexDigest: r.ClientBinary.HashDigest,
	}
	if err := common.ValidateObjectRef(ref, common.KnownHash); err != nil {
		return nil, err
	}
	return ref, nil
}

// ObjectRefAliases is list of ObjectRefs calculated using all hash algos known
// to the server when the client binary was extracted.
//
// Additionally all algos not understood by the server right NOW are skipped
// too. This may arise if the server was rolled back, but some files have
// already been uploaded with a newer algo.
func (r *ClientExtractorResult) ObjectRefAliases() []*api.ObjectRef {
	all := r.ClientBinary.AllHashDigests

	// Older entries do not have AllHashDigests field at all.
	if len(all) == 0 {
		ref := &api.ObjectRef{
			HashAlgo:  api.HashAlgo(api.HashAlgo_value[r.ClientBinary.HashAlgo]),
			HexDigest: r.ClientBinary.HashDigest,
		}
		if common.ValidateObjectRef(ref, common.KnownHash) == nil {
			return []*api.ObjectRef{ref}
		}
		return nil // welp, have 0 supported algos, should not really happen
	}

	// Order the result by HashAlgo enum values. This loop also naturally skips
	// algos not understood by the current version of the server, since they are
	// not in HashAlgo_name map.
	refs := make([]*api.ObjectRef, 0, len(all))
	for algo := int32(1); api.HashAlgo_name[algo] != ""; algo++ { // skip UNSPECIFIED
		if digest := all[api.HashAlgo_name[algo]]; digest != "" {
			ref := &api.ObjectRef{HashAlgo: api.HashAlgo(algo), HexDigest: digest}
			if common.ValidateObjectRef(ref, common.KnownHash) == nil {
				refs = append(refs, ref)
			}
		}
	}
	return refs
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

	// We use same hash algo for naming the extracted file as was used to name
	// the package instance it is in. This avoid some confusion during the
	// transition to a new hash.
	if err = common.ValidateInstanceID(inst.InstanceID, common.KnownHash); err != nil {
		err = errors.Annotate(err, "unrecognized client instance ID format").Err()
		return
	}
	instRef := common.InstanceIDToObjectRef(inst.InstanceID)

	// We also always calculate all other hashes we know about at the same time,
	// for old bootstrap scripts that may not understand the most recent hash
	// algo.
	hashes := make(map[api.HashAlgo]hash.Hash, len(api.HashAlgo_name))
	for algo := range api.HashAlgo_name {
		if a := api.HashAlgo(algo); a != api.HashAlgo_HASH_ALGO_UNSPECIFIED {
			hashes[a] = common.MustNewHash(a)
		}
	}
	if hashes[instRef.HashAlgo] == nil {
		panic("impossible, inst.InstanceID has already been validated")
	}

	// Start extracting the file.
	reader, size, err := pkg.Open(GetClientBinaryName(inst.Package.StringID()))
	if err != nil {
		err = errors.Annotate(err, "failed to open the file for reading").Err()
		return
	}
	defer reader.Close() // we don't care about errors here

	// Start writing the result to CAS.
	op, err := e.CAS.BeginUpload(ctx, &api.BeginUploadRequest{
		HashAlgo: instRef.HashAlgo,
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
	extractedRef := &api.ObjectRef{
		HashAlgo:  instRef.HashAlgo,
		HexDigest: common.HexDigest(hashes[instRef.HashAlgo]),
	}
	op, err = e.CAS.FinishUpload(ctx, &api.FinishUploadRequest{
		UploadOperationId: op.OperationId,
		ForceHash:         extractedRef,
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

	hexDigests := make(map[string]string, len(hashes))
	for algo, hash := range hashes {
		hexDigests[algo.String()] = common.HexDigest(hash)
	}

	r := ClientExtractorResult{}
	r.ClientBinary.Size = size
	r.ClientBinary.HashAlgo = extractedRef.HashAlgo.String()
	r.ClientBinary.HashDigest = extractedRef.HexDigest
	r.ClientBinary.AllHashDigests = hexDigests

	logging.Infof(ctx, "Uploaded CIPD client binary %s with %s %s (%d bytes)",
		inst.Package.StringID(), extractedRef.HashAlgo, extractedRef.HexDigest, size)

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
