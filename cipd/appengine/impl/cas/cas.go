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
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/bqlog"
	"go.chromium.org/luci/server/tq"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas/tasks"
	"go.chromium.org/luci/cipd/appengine/impl/cas/upload"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/monitoring"
	"go.chromium.org/luci/cipd/appengine/impl/settings"
	"go.chromium.org/luci/cipd/common"
)

// readBufferSize is size of a buffer used to read Google Storage files.
//
// Larger values mean fewer Google Storage RPC calls, but more memory usage.
const readBufferSize = 64 * 1024 * 1024

// minimumSpeedLimit is the minimum tolerable speed for GCS readers in
// bytes/sec.
//
// In 25Q1 the typical good speed observed was 5 times this limit (50MB/s).
const minimumSpeedLimit = 10 * 1024 * 1024

// StorageServer extends StorageServer RPC interface with some methods used
// internally by other CIPD server modules.
type StorageServer interface {
	api.StorageServer

	// GetReader returns an io.ReaderAt implementation to read contents of an
	// object in the storage.
	//
	// Returns grpc errors. In particular NotFound is returned if there's no such
	// object in the storage.
	GetReader(ctx context.Context, ref *api.ObjectRef) (gs.Reader, error)
}

// Internal returns non-ACLed implementation of StorageService.
//
// It can be used internally by the backend. Assumes ACL checks are already
// done.
//
// Registers some task queue tasks in the given dispatcher and log sinks in
// the given bundler.
func Internal(d *tq.Dispatcher, b *bqlog.Bundler, s *settings.Settings, opts *server.Options) StorageServer {
	impl := &storageImpl{
		tq:             d,
		settings:       s,
		serviceVersion: opts.ImageVersion(),
		processID:      opts.Hostname,
		getGS:          gs.Get,
		getSignedURL:   getSignedURL,
		submitLog:      func(ctx context.Context, entry *api.VerificationLogEntry) { b.Log(ctx, entry) },
	}
	impl.registerTasks()
	b.RegisterSink(bqlog.Sink{
		Prototype: &api.VerificationLogEntry{},
		Table:     "verification",
	})
	return impl
}

// storageImpl implements api.StorageServer and task queue handlers.
//
// Doesn't do any ACL checks.
type storageImpl struct {
	api.UnimplementedStorageServer

	tq       *tq.Dispatcher
	settings *settings.Settings

	// For VerificationLogEntry fields.
	serviceVersion string
	processID      string

	// Mocking points for tests. See Internal() for real implementations.
	getGS        func(ctx context.Context) gs.GoogleStorage
	getSignedURL func(ctx context.Context, gsPath, filename string, signer signerFactory, gs gs.GoogleStorage) (string, uint64, error)
	submitLog    func(ctx context.Context, entry *api.VerificationLogEntry)
}

// registerTasks adds tasks to the tq Dispatcher.
func (s *storageImpl) registerTasks() {
	// See queue.yaml for "cas-uploads" task queue definition.
	s.tq.RegisterTaskClass(tq.TaskClass{
		ID:        "verify-upload",
		Prototype: &tasks.VerifyUpload{},
		Kind:      tq.Transactional,
		Queue:     "cas-uploads",
		Handler: func(ctx context.Context, m proto.Message) error {
			return s.verifyUploadTask(ctx, m.(*tasks.VerifyUpload))
		},
	})
	s.tq.RegisterTaskClass(tq.TaskClass{
		ID:        "cleanup-upload",
		Prototype: &tasks.CleanupUpload{},
		Kind:      tq.Transactional,
		Queue:     "cas-uploads",
		Handler: func(ctx context.Context, m proto.Message) error {
			return s.cleanupUploadTask(ctx, m.(*tasks.CleanupUpload))
		},
	})
}

// GetReader is part of StorageServer interface.
func (s *storageImpl) GetReader(ctx context.Context, ref *api.ObjectRef) (r gs.Reader, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	if err = common.ValidateObjectRef(ref, common.KnownHash); err != nil {
		return nil, errors.Annotate(err, "bad ref").Err()
	}

	r, err = s.getGS(ctx).Reader(ctx, s.settings.ObjectPath(ref), 0, minimumSpeedLimit)
	if err != nil {
		ann := errors.Annotate(err, "can't read the object")
		if gs.StatusCode(err) == http.StatusNotFound {
			ann.Tag(grpcutil.NotFoundTag)
		}
		return nil, ann.Err()
	}
	return r, nil
}

// GetObjectURL implements the corresponding RPC method, see the proto doc.
func (s *storageImpl) GetObjectURL(ctx context.Context, r *api.GetObjectURLRequest) (resp *api.ObjectURL, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	if err := common.ValidateObjectRef(r.Object, common.KnownHash); err != nil {
		return nil, errors.Annotate(err, "bad 'object' field").Err()
	}

	// Lite validation for Content-Disposition header. As long as the filename
	// doesn't have '"' or '\n', we are constructing a valid header. Let the
	// browser do the rest of the validation however it likes.
	if strings.ContainsAny(r.DownloadFilename, "\"\r\n") {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'download_filename' field, contains one of %q", "\"\r\n")
	}

	url, size, err := s.getSignedURL(ctx, s.settings.ObjectPath(r.Object), r.DownloadFilename, defaultSigner, s.getGS(ctx))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get signed URL").Err()
	}
	monitoring.FileSize(ctx, size)
	return &api.ObjectURL{SignedUrl: url}, nil
}

// BeginUpload implements the corresponding RPC method, see the proto doc.
func (s *storageImpl) BeginUpload(ctx context.Context, r *api.BeginUploadRequest) (resp *api.UploadOperation, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Either Object or HashAlgo should be given. If both are, algos must match.
	var hashAlgo api.HashAlgo
	var hexDigest string
	if r.Object != nil {
		if err := common.ValidateObjectRef(r.Object, common.KnownHash); err != nil {
			return nil, errors.Annotate(err, "bad 'object'").Err()
		}
		if r.HashAlgo != 0 && r.HashAlgo != r.Object.HashAlgo {
			return nil, errors.Reason("'hash_algo' and 'object.hash_algo' do not match").
				Tag(grpcutil.InvalidArgumentTag).Err()
		}
		hashAlgo = r.Object.HashAlgo
		hexDigest = r.Object.HexDigest
	} else if err := common.ValidateHashAlgo(r.HashAlgo); err != nil {
		return nil, errors.Annotate(err, "bad 'hash_algo'").Err()
	} else {
		hashAlgo = r.HashAlgo
	}

	gs := s.getGS(ctx)

	// If we know the name of the object being uploaded, check we don't have it
	// in the store already to avoid wasting time uploading it. Note that it is
	// always fine to "overwrite" objects, so if the object appears while the
	// client is still uploading, nothing catastrophic happens, just some time
	// gets wasted.
	if r.Object != nil {
		switch yes, err := gs.Exists(ctx, s.settings.ObjectPath(r.Object)); {
		case err != nil:
			return nil, errors.Annotate(err, "failed to check the object's presence").
				Tag(grpcutil.InternalTag).Err()
		case yes:
			return nil, status.Errorf(codes.AlreadyExists, "the object is already in the store")
		}
	}

	// Grab new unique ID for the upload operation, it is used in GS filenames.
	opID, err := upload.NewOpID(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to allocate upload operation ID").
			Tag(grpcutil.InternalTag).Err()
	}

	// Attach HMAC to it, to be returned to the client to make sure clients can't
	// access sessions they don't own. Do it early, to avoid storing stuff in
	// the datastore and GS if WrapOpID fails.
	caller := auth.CurrentIdentity(ctx)
	wrappedOpID, err := upload.WrapOpID(ctx, opID, caller)
	if err != nil {
		return nil, errors.Annotate(err, "failed to HMAC-tag upload operation ID").
			Tag(grpcutil.InternalTag).Err()
	}

	// GS path to which the client will upload the data. Prefix it with the
	// current timestamp to make bucket listing sorted by time.
	now := clock.Now(ctx)
	tempGSPath := fmt.Sprintf("%s/%d_%d", s.settings.TempGSPath, now.Unix(), opID)

	// Initiate Google Storage resumable upload session to this path. The returned
	// URL can be accessed unauthenticated. The client will use it directly to
	// upload the data. If left open, the GS session eventually expires, so it's
	// not big deal if we loose it (e.g. due to a crash before returning).
	uploadURL, err := gs.StartUpload(ctx, tempGSPath)
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
	if err = datastore.Put(ctx, &op); err != nil {
		return nil, errors.Annotate(err, "failed to persist upload operation").
			Tag(grpcutil.InternalTag).Err()
	}

	return op.ToProto(wrappedOpID), nil
}

// FinishUpload implements the corresponding RPC method, see the proto doc.
func (s *storageImpl) FinishUpload(ctx context.Context, r *api.FinishUploadRequest) (resp *api.UploadOperation, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	if r.ForceHash != nil {
		if err := common.ValidateObjectRef(r.ForceHash, common.KnownHash); err != nil {
			return nil, errors.Annotate(err, "bad 'force_hash' field").Err()
		}
	}

	// Grab the corresponding operation and inspect its status.
	op, err := fetchOp(ctx, r.UploadOperationId)
	switch {
	case err != nil:
		return nil, err
	case op.Status != api.UploadStatus_UPLOADING:
		// Nothing to do if the operation is already closed or being verified.
		return op.ToProto(r.UploadOperationId), nil
	}

	// If the forced hash is provided by the (trusted) caller, we are almost done.
	// Just need to move the temp file to its final location based on this hash
	// and close the operation.
	if r.ForceHash != nil {
		mutated, err := s.finishAndForcedHash(ctx, op, r.ForceHash)
		if err != nil {
			return nil, err
		}
		return mutated.ToProto(r.UploadOperationId), nil
	}

	// Otherwise start the hash verification task, see verifyUploadTask below.
	mutated, err := op.Advance(ctx, func(ctx context.Context, op *upload.Operation) error {
		op.Status = api.UploadStatus_VERIFYING
		return s.tq.AddTask(ctx, &tq.Task{
			Title:   fmt.Sprintf("%d", op.ID),
			Payload: &tasks.VerifyUpload{UploadOperationId: op.ID},
		})
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to start the verification task").
			Tag(grpcutil.InternalTag).Err()
	}
	return mutated.ToProto(r.UploadOperationId), nil
}

// CancelUpload implements the corresponding RPC method, see the proto doc.
func (s *storageImpl) CancelUpload(ctx context.Context, r *api.CancelUploadRequest) (resp *api.UploadOperation, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	handleOpStatus := func(op *upload.Operation) (*api.UploadOperation, error) {
		if op.Status == api.UploadStatus_ERRORED || op.Status == api.UploadStatus_CANCELED {
			return op.ToProto(r.UploadOperationId), nil
		}
		return nil, errors.Reason("the operation is in state %s and can't be canceled", op.Status).Tag(grpcutil.FailedPreconditionTag).Err()
	}

	// Grab the corresponding operation and inspect its status.
	op, err := fetchOp(ctx, r.UploadOperationId)
	switch {
	case err != nil:
		return nil, err
	case op.Status != api.UploadStatus_UPLOADING:
		return handleOpStatus(op)
	}

	// Move the operation to canceled state and launch the TQ task to cleanup.
	mutated, err := op.Advance(ctx, func(ctx context.Context, op *upload.Operation) error {
		op.Status = api.UploadStatus_CANCELED
		return s.tq.AddTask(ctx, &tq.Task{
			Title: fmt.Sprintf("%d", op.ID),
			Payload: &tasks.CleanupUpload{
				UploadOperationId: op.ID,
				UploadUrl:         op.UploadURL,
				PathToCleanup:     op.TempGSPath,
			},
		})
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to start the cleanup task").
			Tag(grpcutil.InternalTag).Err()
	}
	return handleOpStatus(mutated)
}

// fethcOp unwraps upload operation ID and fetches upload.Operation entity.
//
// Returns an grpc-tagged error on failure that can be returned to the RPC
// caller right away.
func fetchOp(ctx context.Context, wrappedOpID string) (*upload.Operation, error) {
	opID, err := upload.UnwrapOpID(ctx, wrappedOpID, auth.CurrentIdentity(ctx))
	if err != nil {
		if transient.Tag.In(err) {
			return nil, errors.Annotate(err, "failed to check HMAC on upload_operation_id").Err()
		}
		logging.Infof(ctx, "HMAC check failed - %s", err)
		return nil, errors.Reason("no such upload operation").Tag(grpcutil.NotFoundTag).Err()
	}

	op := &upload.Operation{ID: opID}
	switch err := datastore.Get(ctx, op); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.Reason("no such upload operation").
			Tag(grpcutil.NotFoundTag).Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch the upload operation").
			Tag(grpcutil.InternalTag).Err()
	}

	return op, nil
}

// finishAndForcedHash finalizes uploads that use ForceHash field.
//
// It publishes the object immediately, skipping the verification.
func (s *storageImpl) finishAndForcedHash(ctx context.Context, op *upload.Operation, hash *api.ObjectRef) (*upload.Operation, error) {
	gs := s.getGS(ctx)

	// Try to move the object into the final location. This may fail
	// transiently, in which case we ask the client to retry, or fatally, in
	// which case we close the upload operation with an error.
	pubErr := gs.Publish(ctx, s.settings.ObjectPath(hash), op.TempGSPath, -1)
	if transient.Tag.In(pubErr) {
		return nil, errors.Annotate(pubErr, "failed to publish the object").
			Tag(grpcutil.InternalTag).Err()
	}

	// Try to remove the leftover garbage. See maybeDelete doc for possible
	// caveats.
	if err := s.maybeDelete(ctx, gs, op.TempGSPath); err != nil {
		return nil, err
	}

	// Set the status of the operation based on whether we published the file
	// or not.
	return op.Advance(ctx, func(_ context.Context, op *upload.Operation) error {
		if pubErr != nil {
			op.Status = api.UploadStatus_ERRORED
			op.Error = fmt.Sprintf("Failed to publish the object - %s", pubErr)
		} else {
			op.Status = api.UploadStatus_PUBLISHED
			op.HashAlgo = hash.HashAlgo
			op.HexDigest = hash.HexDigest
		}
		return nil
	})
}

// verifyUploadTask verifies data uploaded by a user and closes the upload
// operation based on the result.
//
// Returning a transient error here causes the task queue service to retry the
// task.
func (s *storageImpl) verifyUploadTask(ctx context.Context, task *tasks.VerifyUpload) (err error) {
	op := &upload.Operation{ID: task.UploadOperationId}
	switch err := datastore.Get(ctx, op); {
	case err == datastore.ErrNoSuchEntity:
		return errors.Reason("no such upload operation %d", op.ID).Err()
	case err != nil:
		return errors.Annotate(err, "failed to fetch upload operation %d", op.ID).
			Tag(transient.Tag).Err()
	case op.Status != api.UploadStatus_VERIFYING:
		logging.Infof(ctx, "The upload operation %d is not pending verification anymore (status = %s)", op.ID, op.Status)
		return nil
	}

	gs := s.getGS(ctx)

	// If the destination file exists already, we are done. This may happen on
	// a task retry or if the file was uploaded concurrently by someone else.
	// Otherwise we still need to verify the temp file, and then move it into
	// the final location.
	if op.HexDigest != "" {
		exists, err := gs.Exists(ctx, s.settings.ObjectPath(&api.ObjectRef{
			HashAlgo:  op.HashAlgo,
			HexDigest: op.HexDigest,
		}))
		switch {
		case err != nil:
			return errors.Annotate(err, "failed to check the presence of the destination file").
				Tag(transient.Tag).Err()
		case exists:
			if err := s.maybeDelete(ctx, gs, op.TempGSPath); err != nil {
				return err
			}
			_, err = op.Advance(ctx, func(_ context.Context, op *upload.Operation) error {
				op.Status = api.UploadStatus_PUBLISHED
				return nil
			})
			return err
		}
	}

	verifiedHexDigest := "" // set after the successful hash verification below

	// Log some details about the verification operation.
	logEntry := &api.VerificationLogEntry{
		OperationId:    op.ID,
		InitiatedBy:    string(op.CreatedBy),
		TempGsPath:     op.TempGSPath,
		Submitted:      op.CreatedTS.UnixNano() / 1000,
		Started:        clock.Now(ctx).UnixNano() / 1000,
		ServiceVersion: s.serviceVersion,
		ProcessId:      s.processID,
		TraceId:        trace.SpanContextFromContext(ctx).TraceID().String(),
	}
	if op.HexDigest != "" {
		logEntry.ExpectedInstanceId = common.ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  op.HashAlgo,
			HexDigest: op.HexDigest,
		})
	}

	submitLog := func(outcome api.UploadStatus, error string) {
		logEntry.Outcome = outcome.String()
		logEntry.Error = error
		logEntry.Finished = clock.Now(ctx).UnixNano() / 1000

		verificationTimeSec := float64(logEntry.Finished-logEntry.Started) / 1e6
		if verificationTimeSec < 0.001 {
			verificationTimeSec = 0.001
		}
		logEntry.VerificationSpeed = int64(float64(logEntry.FileSize) / verificationTimeSec)

		if s.submitLog != nil {
			s.submitLog(ctx, logEntry)
		}
	}

	defer func() {
		if err != nil {
			logging.Errorf(ctx, "Verification error: %s", err)
		}

		// On transient errors don't touch the temp file or the operation, we need
		// them for retries.
		if transient.Tag.In(err) {
			submitLog(api.UploadStatus_ERRORED, fmt.Sprintf("Transient error: %s", err))
			return
		}

		// Update the status of the operation based on 'err'. If Advance fails
		// itself, return a transient error to make sure 'verifyUploadTask' is
		// retried.
		advancedOp, opErr := op.Advance(ctx, func(_ context.Context, op *upload.Operation) error {
			if err != nil {
				op.Status = api.UploadStatus_ERRORED
				op.Error = fmt.Sprintf("Verification failed: %s", err)
			} else {
				op.Status = api.UploadStatus_PUBLISHED
				op.HexDigest = verifiedHexDigest
			}
			return nil
		})
		if opErr != nil {
			err = opErr // override the error returned by the task
			submitLog(api.UploadStatus_ERRORED, fmt.Sprintf("Error updating UploadOperation: %s", err))
			return
		}

		submitLog(advancedOp.Status, advancedOp.Error)

		// Best effort deletion of the temporary file. We do it here, after updating
		// the operation, to avoid retrying the expensive verification procedure
		// just because Delete is flaky. Having a little garbage in the temporary
		// directory doesn't hurt (it is marked with operation ID and timestamp,
		// so we can always clean it up offline).
		if delErr := gs.Delete(ctx, op.TempGSPath); delErr != nil {
			logging.WithError(delErr).Errorf(ctx,
				"Failed to remove temporary Google Storage file, it is dead garbage now: %s", op.TempGSPath)
		}
	}()

	hash, err := common.NewHash(op.HashAlgo)
	if err != nil {
		return err
	}

	// Prepare reading the most recent generation of the uploaded temporary file.
	r, err := gs.Reader(ctx, op.TempGSPath, 0, minimumSpeedLimit)
	if err != nil {
		return errors.Annotate(err, "failed to start reading Google Storage file").Err()
	}

	// Pick large buffer to reduce number of Google Storage RPC calls. Don't
	// allocate more than necessary though.
	fileSize := r.Size()
	bufSize := readBufferSize
	if fileSize < int64(bufSize) {
		bufSize = int(fileSize)
	}
	logEntry.FileSize = fileSize

	// Feed the file to the hasher.
	_, err = io.CopyBuffer(hash, io.NewSectionReader(r, 0, fileSize), make([]byte, bufSize))
	if err != nil {
		return errors.Annotate(err, "failed to read Google Storage file").Err()
	}
	verifiedHexDigest = hex.EncodeToString(hash.Sum(nil))

	// This should usually match logEntry.ExpectedInstanceId.
	logEntry.VerifiedInstanceId = common.ObjectRefToInstanceID(&api.ObjectRef{
		HashAlgo:  op.HashAlgo,
		HexDigest: verifiedHexDigest,
	})

	// If we know the expected hash, verify it matches what we have calculated.
	if op.HexDigest != "" && op.HexDigest != verifiedHexDigest {
		return errors.Reason("expected %s to be %s, got %s", op.HashAlgo, op.HexDigest, verifiedHexDigest).Err()
	}

	// The verification was successful, move the temp file (at the generation we
	// have just verified) to the final location. If the file was modified after
	// we have verified it (has different generation number), Publish fails:
	// clients must not modify uploads after calling FinishUpload, this is
	// sneaky behavior. Regardless of the outcome of this operation, the upload
	// operation is closed in the defer above.
	err = gs.Publish(ctx, s.settings.ObjectPath(&api.ObjectRef{
		HashAlgo:  op.HashAlgo,
		HexDigest: verifiedHexDigest,
	}), op.TempGSPath, r.Generation())
	if err != nil {
		return errors.Annotate(err, "failed to publish the verified file").Err()
	}
	return nil
}

// cleanupUploadTask is called to clean up after a canceled upload.
//
// Best effort. If the temporary file can't be deleted from GS due to some
// non-transient error, logs the error and ignores it, since retrying won't
// help.
func (s *storageImpl) cleanupUploadTask(ctx context.Context, task *tasks.CleanupUpload) (err error) {
	gs := s.getGS(ctx)

	if err := gs.CancelUpload(ctx, task.UploadUrl); err != nil {
		if transient.Tag.In(err) {
			return errors.Annotate(err, "transient error when canceling the resumable upload").Err()
		}
		logging.WithError(err).Errorf(ctx, "Failed to cancel resumable upload")
	}

	if err := gs.Delete(ctx, task.PathToCleanup); err != nil {
		if transient.Tag.In(err) {
			return errors.Annotate(err, "transient error when deleting the temp file").Err()
		}
		logging.WithError(err).Errorf(ctx, "Failed to delete the temp file")
	}

	return nil
}

// maybeDelete is called to delete temporary file when finishing an upload.
//
// If this fails transiently, we ask the client (or the task queue) to retry the
// corresponding RPC (by returning transient errors), so the file is deleted
// eventually. It means Publish may be called again too, but it is idempotent,
// so it is fine.
//
// If Delete fails fatally, we are in a tough position, since we did publish the
// file already, so the upload operation is technically successful and marking
// it as failed is a lie. So we log and ignore fatal Delete errors. They should
// not happen anyway.
//
// Thus, this function returns either nil or a transient error.
func (s *storageImpl) maybeDelete(ctx context.Context, gs gs.GoogleStorage, path string) error {
	switch err := gs.Delete(ctx, path); {
	case transient.Tag.In(err):
		return errors.Annotate(err, "transient error when removing temporary Google Storage file").
			Tag(grpcutil.InternalTag).Err()
	case err != nil:
		logging.WithError(err).Errorf(ctx, "Failed to remove temporary Google Storage file, it is dead garbage now: %s", path)
	}
	return nil
}
