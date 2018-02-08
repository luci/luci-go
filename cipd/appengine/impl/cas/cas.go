// Copyright 2017 The LUCI Authors.
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

package cas

import (
	"fmt"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas/upload"
	"go.chromium.org/luci/cipd/appengine/impl/common"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/settings"
)

// Internal returns non-ACLed implementation of cas.StorageService.
//
// It can be used internally by the backend. Assumes ACL checks are already
// done.
func Internal() api.StorageServer {
	return &storageImpl{
		getGS:        gs.Get,
		settings:     settings.Get,
		getSignedURL: getSignedURL,
	}
}

// storageImpl implements api.StorageServer.
//
// Doesn't do any ACL checks.
type storageImpl struct {
	// Mocking points for tests. See Internal() for real implementations.
	getGS        func(c context.Context) gs.GoogleStorage
	settings     func(c context.Context) (*settings.Settings, error)
	getSignedURL func(c context.Context, gsPath, filename string, signer signerFactory, gs gs.GoogleStorage) (string, error)
}

// GetObjectURL implements the corresponding RPC method, see the proto doc.
func (s *storageImpl) GetObjectURL(c context.Context, r *api.GetObjectURLRequest) (resp *api.ObjectURL, err error) {
	defer func() { err = common.GRPCifyAndLogErr(c, err) }()

	if err := ValidateObjectRef(r.Object); err != nil {
		return nil, errors.Annotate(err, "bad 'object' field").Err()
	}

	// Lite validation for Content-Disposition header. As long as the filename
	// doesn't have '"' or '\n', we are constructing a valid header. Let the
	// browser do the rest of the validation however it likes.
	if strings.ContainsAny(r.DownloadFilename, "\"\r\n") {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'download_filename' field, contains one of %q", "\"\r\n")
	}

	cfg, err := s.settings(c)
	if err != nil {
		return nil, err
	}

	sigFactory := defaultSigner
	if cfg.SignAs != "" {
		sigFactory = func(c context.Context) (*signer, error) {
			return iamSigner(c, cfg.SignAs)
		}
	}

	url, err := s.getSignedURL(c, cfg.ObjectPath(r.Object), r.DownloadFilename, sigFactory, s.getGS(c))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get signed URL").Err()
	}
	return &api.ObjectURL{SignedUrl: url}, nil
}

// BeginUpload implements the corresponding RPC method, see the proto doc.
func (s *storageImpl) BeginUpload(c context.Context, r *api.BeginUploadRequest) (resp *api.UploadOperation, err error) {
	defer func() { err = common.GRPCifyAndLogErr(c, err) }()

	// Either Object or HashAlgo should be given. If both are, algos must match.
	var hashAlgo api.HashAlgo
	var hexDigest string
	if r.Object != nil {
		if err := ValidateObjectRef(r.Object); err != nil {
			return nil, errors.Annotate(err, "bad 'object'").Err()
		}
		if r.HashAlgo != 0 && r.HashAlgo != r.Object.HashAlgo {
			return nil, errors.Reason("'hash_algo' and 'object.hash_algo' do not match").
				Tag(grpcutil.InvalidArgumentTag).Err()
		}
		hashAlgo = r.Object.HashAlgo
		hexDigest = r.Object.HexDigest
	} else if err := ValidateHashAlgo(r.HashAlgo); err != nil {
		return nil, errors.Annotate(err, "bad 'hash_algo'").Err()
	} else {
		hashAlgo = r.HashAlgo
	}

	gs := s.getGS(c)
	cfg, err := s.settings(c)
	if err != nil {
		return nil, err
	}

	// If we know the name of the object being uploaded, check we don't have it
	// in the store already to avoid wasting time uploading it. Note that it is
	// always fine to "overwrite" objects, so if the object appears while the
	// client is still uploading, nothing catastrophic happens, just some time
	// gets wasted.
	if r.Object != nil {
		switch yes, err := gs.Exists(c, cfg.ObjectPath(r.Object)); {
		case err != nil:
			return nil, errors.Annotate(err, "failed to check the object's presence").
				Tag(grpcutil.InternalTag).Err()
		case yes:
			return nil, status.Errorf(codes.AlreadyExists, "the object is already in the store")
		}
	}

	// Grab new unique ID for the upload operation, it is used in GS filenames.
	opID, err := upload.NewOpID(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to allocate upload operation ID").
			Tag(grpcutil.InternalTag).Err()
	}

	// Attach HMAC to it, to be returned to the client to make sure clients can't
	// access sessions they don't own. Do it early, to avoid storing stuff in
	// the datastore and GS if WrapOpID fails.
	caller := auth.CurrentIdentity(c)
	wrappedOpID, err := upload.WrapOpID(c, opID, caller)
	if err != nil {
		return nil, errors.Annotate(err, "failed to HMAC-tag upload operation ID").
			Tag(grpcutil.InternalTag).Err()
	}

	// GS path to which the client will upload the data. Prefix it with the
	// current timestamp to make bucket listing sorted by time.
	now := clock.Now(c)
	tempGSPath := fmt.Sprintf("%s/%d_%d", cfg.TempGSPath, now.Unix(), opID)

	// Initiate Google Storage resumable upload session to this path. The returned
	// URL can be accessed unauthenticated. The client will use it directly to
	// upload the data. If left open, the GS session eventually expires, so it's
	// not big deal if we loose it (e.g due to a crash before returning).
	uploadURL, err := gs.StartUpload(c, tempGSPath)
	if err != nil {
		return nil, errors.Annotate(err, "failed to start resumable upload").
			Tag(grpcutil.InternalTag).Err()
	}

	// Save the operation. It is accessed in FinishUpload.
	op := upload.Operation{
		ID:         opID,
		Status:     api.UploadStatus_UPLOADING,
		TempGSPath: tempGSPath,
		UploadURL:  uploadURL,
		HashAlgo:   hashAlgo,
		HexDigest:  hexDigest, // may be empty, means the server should calculate it
		CreatedBy:  caller,
		CreatedTS:  now.UTC(),
		UpdatedTS:  now.UTC(),
	}
	if err = datastore.Put(c, &op); err != nil {
		return nil, errors.Annotate(err, "failed to persist upload operation").
			Tag(grpcutil.InternalTag).Err()
	}

	return op.ToProto(wrappedOpID), nil
}

// FinishUpload implements the corresponding RPC method, see the proto doc.
func (s *storageImpl) FinishUpload(c context.Context, r *api.FinishUploadRequest) (resp *api.UploadOperation, err error) {
	defer func() { err = common.GRPCifyAndLogErr(c, err) }()

	if r.ForceHash != nil {
		if err := ValidateObjectRef(r.ForceHash); err != nil {
			return nil, errors.Annotate(err, "bad 'force_hash' field").Err()
		}
	}

	// TODO(vadimsh): Implement.

	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}
