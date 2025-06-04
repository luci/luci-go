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
	"io"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
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
func (e *ClientExtractor) Applicable(ctx context.Context, inst *model.Instance) (bool, error) {
	return IsClientPackage(inst.Package.StringID()), nil
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
		err = errors.Fmt("unrecognized client instance ID format: %w", err)
		return
	}
	instRef := common.InstanceIDToObjectRef(inst.InstanceID)

	// We also always calculate all other hashes we know about at the same time,
	// for old bootstrap scripts that may not understand the most recent hash
	// algo.
	hashes := make([]api.HashAlgo, 0, len(api.HashAlgo_name))
	for algo := range api.HashAlgo_name {
		if a := api.HashAlgo(algo); a != api.HashAlgo_HASH_ALGO_UNSPECIFIED {
			hashes = append(hashes, a)
		}
	}

	// Execute the extraction.
	result, err := (&Extractor{
		Reader:            pkg,
		CAS:               e.CAS,
		PrimaryHash:       instRef.HashAlgo,
		AlternativeHashes: hashes,
		Uploader:          e.uploader,
		BufferSize:        e.bufferSize,
	}).Run(ctx, GetClientBinaryName(inst.Package.StringID()))
	if err != nil {
		return
	}

	// Store the results in the appropriate format.
	hexDigests := make(map[string]string, len(result.Hashes))
	for algo, hash := range result.Hashes {
		hexDigests[algo.String()] = common.HexDigest(hash)
	}

	r := ClientExtractorResult{}
	r.ClientBinary.Size = result.Size
	r.ClientBinary.HashAlgo = result.Ref.HashAlgo.String()
	r.ClientBinary.HashDigest = result.Ref.HexDigest
	r.ClientBinary.AllHashDigests = hexDigests

	logging.Infof(ctx, "Uploaded CIPD client binary %s with %s %s (%d bytes)",
		inst.Package.StringID(), result.Ref.HashAlgo, result.Ref.HexDigest, result.Size)

	res.Result = r
	return
}

// GetClientExtractorResult returns results of client extractor processor.
//
// They contain a reference to the unpacked CIPD binary object in the Google
// Storage.
//
// Returns:
//
//	(result, nil) on success.
//	(nil, datastore.ErrNoSuchEntity) if results are not available.
//	(nil, transient-tagged error) on retrieval errors.
//	(nil, non-transient-tagged error) if the client extractor failed.
func GetClientExtractorResult(ctx context.Context, inst *api.Instance) (*ClientExtractorResult, error) {
	r := &model.ProcessingResult{
		ProcID:   ClientExtractorProcID,
		Instance: datastore.KeyForObj(ctx, (&model.Instance{}).FromProto(ctx, inst)),
	}
	switch err := datastore.Get(ctx, r); {
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	case err != nil:
		return nil, transient.Tag.Apply(err)
	case !r.Success:
		return nil, errors.Fmt("client extraction failed: %s", r.Error)
	}
	out := &ClientExtractorResult{}
	if err := r.ReadResult(out); err != nil {
		return nil, errors.Fmt("failed to parse the client extractor status: %w", err)
	}
	return out, nil
}
