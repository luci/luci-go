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
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas/tasks"
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
	return impl
}

// impl is the actual real implementation of api.StorageServer.
var impl = &storageImpl{
	getGS:        gs.Get,
	settings:     settings.Get,
	getSignedURL: getSignedURL,
}

func init() {
	// See queue.yaml for "verify" task queue definition.
	common.TQ.RegisterTask(&tasks.VerifyUpload{}, func(c context.Context, m proto.Message) error {
		return impl.verifyUploadTask(c, m.(*tasks.VerifyUpload))
	}, "verify", nil)
}

// storageImpl implements api.StorageServer and task queue handlers.
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

	// Verify HMAC of the upload operation ID.
	opID, err := upload.UnwrapOpID(c, r.UploadOperationId, auth.CurrentIdentity(c))
	if err != nil {
		if transient.Tag.In(err) {
			return nil, errors.Annotate(err, "failed to check HMAC on upload_operation_id").Err()
		}
		return nil, errors.Reason("no such upload operation").
			InternalReason("HMAC check failed - %s", err).
			Tag(grpcutil.NotFoundTag).Err()
	}

	// Grab the corresponding operation to inspect its status.
	op := upload.Operation{ID: opID}
	if err := datastore.Get(c, &op); err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, errors.Reason("no such upload operation").
				Tag(grpcutil.NotFoundTag).Err()
		}
		return nil, errors.Annotate(err, "failed to fetch upload operation").
			Tag(grpcutil.InternalTag).Err()
	}

	// Nothing to do if the operation is already closed or being verified.
	if op.Status != api.UploadStatus_UPLOADING {
		return op.ToProto(r.UploadOperationId), nil
	}

	// If the forced hash is provided by the (trusted) caller, we are almost done.
	// Just need to move the temp file to its final location based on this hash
	// and close the operation.
	if r.ForceHash != nil {
		gs := s.getGS(c)
		cfg, err := s.settings(c)
		if err != nil {
			return nil, err
		}
		// Try to move the object into the final location. This may fail
		// transiently, in which case we ask the client to retry, or fatally, in
		// which case we close the upload operation with an error.
		pubErr := gs.Publish(c, op.TempGSPath, cfg.ObjectPath(r.ForceHash), -1)
		if transient.Tag.In(pubErr) {
			return nil, errors.Annotate(pubErr, "failed to publish the object").
				Tag(grpcutil.InternalTag).Err()
		}
		mutated, err := op.Finish(c, gs, func(_ context.Context, op *upload.Operation) error {
			if pubErr != nil {
				op.Status = api.UploadStatus_ERRORED
				op.Error = fmt.Sprintf("Failed to publish the object - %s", pubErr)
			} else {
				op.Status = api.UploadStatus_PUBLISHED
				op.HashAlgo = r.ForceHash.HashAlgo
				op.HexDigest = r.ForceHash.HexDigest
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return mutated.ToProto(r.UploadOperationId), nil
	}

	// Otherwise start the hash verification task, see verifyUploadTask below.
	mutated, err := op.Advance(c, func(c context.Context, op *upload.Operation) error {
		op.Status = api.UploadStatus_VERIFYING
		return common.TQ.AddTask(c, &tq.Task{
			Payload: &tasks.VerifyUpload{UploadOperationId: opID},
			Title:   fmt.Sprintf("%d", opID),
		})
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to start the verification task").
			Tag(grpcutil.InternalTag).Err()
	}
	return mutated.ToProto(r.UploadOperationId), nil
}

// verifyUploadTask verifies data uploaded by a user and closes the upload
// operation based on the result.
//
// Returning a transient error here causes the task queue service to retry the
// task.
func (s *storageImpl) verifyUploadTask(c context.Context, task *tasks.VerifyUpload) (err error) {
	op := &upload.Operation{ID: task.UploadOperationId}
	if err := datastore.Get(c, op); err != nil {
		if err == datastore.ErrNoSuchEntity {
			return errors.Reason("no such upload operation %d", op.ID).Err()
		}
		return errors.Annotate(err, "failed to fetch upload operation %d", op.ID).
			Tag(transient.Tag).Err()
	}
	if op.Status != api.UploadStatus_VERIFYING {
		logging.Infof(c, "The upload operation %d is not pending verification (status = %s)", op.ID, op.Status)
		return nil
	}

	gsImpl := s.getGS(c)
	cfg, err := s.settings(c)
	if err != nil {
		return err
	}

	// If the destination file exists already, we are done. This may happen on
	// a task retry or if the file was uploaded concurrently by someone else.
	// Otherwise we still need to verify the temp file, and then move it into
	// the final location.
	if op.HexDigest != "" {
		exists, err := gsImpl.Exists(c, cfg.ObjectPath(&api.ObjectRef{
			HashAlgo:  op.HashAlgo,
			HexDigest: op.HexDigest,
		}))
		switch {
		case err != nil:
			return errors.Annotate(err, "failed to check the presence of the destination file").
				Tag(transient.Tag).Err()
		case exists:
			_, err = op.Finish(c, gsImpl, func(_ context.Context, op *upload.Operation) error {
				op.Status = api.UploadStatus_PUBLISHED
				return nil
			})
			return err
		}
	}

	defer func() {
		if err == nil || transient.Tag.In(err) {
			return
		}
		// On fatal errors mark the operation as failed. If this change fails
		// itself, return a transient error to make sure 'verifyUploadTask' is
		// retried. Note that at this point the temp file may be deleted already and
		// the retry will also fail (fatally) in ReadAll, which is what we want.
		// Albeit the verification error will be different, but this condition
		// should be rare, so this is fine.
		_, finishErr := op.Finish(c, gsImpl, func(_ context.Context, op *upload.Operation) error {
			op.Status = api.UploadStatus_ERRORED
			op.Error = fmt.Sprintf("Verification failed - %s", err)
			return nil
		})
		if finishErr != nil {
			err = errors.Annotate(finishErr, "failed to update the operation after a verification error %s", err).Err()
		}
	}()

	// Read the most recent generation of the file and feed it to the hasher.
	hasher, err := NewHash(op.HashAlgo)
	if err != nil {
		return err
	}
	gen, err := gs.ReadAll(c, gsImpl, op.TempGSPath, 4*1024*1024, hasher)
	if err != nil {
		return err
	}
	hexDigest := hex.EncodeToString(hasher.Sum(nil))

	// If we know the expected hash, verify it matches what we have calculated.
	if op.HexDigest != "" && op.HexDigest != hexDigest {
		return errors.Reason("expected %s to be %s, got %s", op.HashAlgo, op.HexDigest, hexDigest).Err()
	}

	// The verification was successful, move the temp file (at the generation we
	// have just verified) to the final location. If the file was modified after
	// we have verified it (has different generation number), Publish fails and
	// we close the upload operation as failed. Clients must not modify uploads
	// after calling FinishUpload.
	err = gsImpl.Publish(c, op.TempGSPath, cfg.ObjectPath(&api.ObjectRef{
		HashAlgo:  op.HashAlgo,
		HexDigest: hexDigest,
	}), gen)
	if err != nil {
		return errors.Annotate(err, "failed to publish the verified file").Err()
	}
	_, err = op.Finish(c, gsImpl, func(_ context.Context, op *upload.Operation) error {
		op.Status = api.UploadStatus_PUBLISHED
		op.HexDigest = hexDigest
		return nil
	})
	return err
}
