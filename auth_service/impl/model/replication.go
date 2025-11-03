// Copyright 2022 The LUCI Authors.
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

package model

import (
	"context"
	"crypto/sha512"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/taskspb"
	"go.chromium.org/luci/auth_service/internal/gs"
	"go.chromium.org/luci/auth_service/internal/pubsub"
)

// EnqueueReplicationTask adds a ReplicationTask task to the cloud task
// queue.
func EnqueueReplicationTask(ctx context.Context, authdbrev int64) error {
	if authdbrev < 0 {
		return errors.New("negative revision numbers are not allowed")
	}
	logging.Debugf(ctx, "enqueuing ReplicationTask %d", authdbrev)
	return tq.AddTask(ctx, &tq.Task{
		Payload: &taskspb.ReplicationTask{AuthDbRev: authdbrev},
		Title:   fmt.Sprintf("authdb-rev-%d", authdbrev),
	})
}

func handleReplicationTask(ctx context.Context, task *taskspb.ReplicationTask) error {
	authDBRev := task.GetAuthDbRev()
	logging.Infof(ctx, "replicating AuthDB for Rev %d", authDBRev)

	if err := replicate(ctx, authDBRev); err != nil {
		if transient.Tag.In(err) {
			// Return the error to signal retry.
			return err
		}

		// Error is non-transient; do not retry.
		logging.Errorf(ctx, "error replicating AuthDB: %s", err)
		return tq.Fatal.Apply(err)
	}
	return nil
}

// replicate triggers AuthDB replication for all client types.
//
// Note: to avoid stale tasks, it will first check that the
// AuthReplicationState.AuthDBRev is still equal to the
// given authDBRev before doing anthing.
func replicate(ctx context.Context, authDBRev int64) error {
	replicationState, err := GetReplicationState(ctx)
	if err != nil {
		return errors.Fmt("failed to get current replication state: %w", err)
	}

	// Check the task is not stale before doing any heavy lifting.
	if replicationState.AuthDBRev != authDBRev {
		logging.Infof(ctx, "skipping stale replication task; requested Rev %d but current Rev is already %d",
			authDBRev, replicationState.AuthDBRev)
		return nil
	}

	// Pack the entire AuthDB into a blob to be stored in the datastore,
	// Google Storage and directly pushed to Replicas.
	replicationState, revisionInfo, authDBBlob, err := packAuthDB(ctx)
	if err != nil {
		return errors.Fmt("failed to pack AuthDB: %w", err)
	}

	// Put the blob into datastore. Also updates pointer to the latest
	// stored blob. This is used by the endpoint at
	// /auth_service/api/v1/authdb/revisions/<rev|"latest">
	if err := StoreAuthDBSnapshot(ctx, replicationState, authDBBlob); err != nil {
		return errors.Fmt("failed to store AuthDBSnapshot to datastore: %w", err)
	}

	// Sign the blob, so even if it travels through an unprotected
	// channel, consumers can still verify that it was produced by us.
	signer := auth.GetSigner(ctx)
	if signer == nil {
		return errors.New("no signer - aborting AuthDB replication after storing AuthDBSnapshot")
	}
	blobChecksum := sha512.Sum512(authDBBlob)
	blobDigest := blobChecksum[:]
	keyName, sig, err := signer.SignBytes(ctx, blobDigest)
	if err != nil {
		return errors.Fmt("error signing AuthDB: %w", err)
	}

	// Put the blob into Google Storage, if the path has been configured.
	gsPath, err := gs.GetPath(ctx)
	if err == nil && gs.IsValidPath(gsPath) {
		// Upload to GS.
		err := uploadToGS(ctx, replicationState, authDBBlob, sig, keyName)
		if err != nil {
			logging.Errorf(ctx, "failed to upload AuthDB Rev %d to GS: %s", authDBRev, err)
			return err
		}
	}

	// Notify PubSub subscribers that a new snapshot is available.
	if err := pubsub.PublishAuthDBRevision(ctx, revisionInfo); err != nil {
		logging.Errorf(ctx, "error publishing PubSub message for revision %d: %s",
			revisionInfo.AuthDbRev, err)
		return err
	}

	return nil
}

// packAuthDB packs the AuthDB into a blob (serialized proto message).
//
// Returns:
//   - replicationState: the AuthReplicationState corresponding to the
//     returned authDBBlob.
//   - authDBRevision: the AuthDBRevision corresponding to the returned
//     authDBBlob.
//   - authDBBlob: serialized protocol.ReplicationPushRequest message
//     (has AuthDB inside).
func packAuthDB(ctx context.Context) (*AuthReplicationState, *protocol.AuthDBRevision, []byte, error) {
	// Get the replication state.
	replicationState, err := GetReplicationState(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	authDBRevision := &protocol.AuthDBRevision{
		PrimaryId:  info.AppID(ctx),
		AuthDbRev:  replicationState.AuthDBRev,
		ModifiedTs: replicationState.ModifiedTS.UnixMicro(),
	}

	// Take a snapshot of the AuthDB.
	snapshot, err := TakeSnapshot(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	authDBProto, err := snapshot.ToAuthDBProto(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	// Validate the AuthDB and log how long it took.
	start := time.Now()
	err = validateAuthDB(ctx, authDBProto)
	logging.Debugf(ctx, "validating AuthDB took %.2f seconds", time.Since(start).Seconds())
	if err != nil {
		return nil, nil, nil, err
	}

	// Serialize to binary proto message.
	req := &protocol.ReplicationPushRequest{
		Revision:        authDBRevision,
		AuthDb:          authDBProto,
		AuthCodeVersion: AuthAPIVersion,
	}
	blob, err := proto.Marshal(req)
	if err != nil {
		return nil, nil, nil, err
	}

	return replicationState, authDBRevision, blob, nil
}

func uploadToGS(ctx context.Context, replicationState *AuthReplicationState, authDBBlob, sig []byte, keyName string) error {
	readers, err := GetAuthorizedEmails(ctx)
	if err != nil {
		return errors.Fmt("error getting authorized reader emails: %w", err)
	}

	serviceAccount, err := getServiceAccountName(ctx)
	if err != nil {
		return errors.Fmt("error getting service account for signed AuthDB: %w", err)
	}
	signedAuthDB := &protocol.SignedAuthDB{
		AuthDbBlob:   authDBBlob,
		SignerId:     serviceAccount,
		SigningKeyId: keyName,
		Signature:    sig,
	}
	authDBRevision := &protocol.AuthDBRevision{
		PrimaryId:  info.AppID(ctx),
		AuthDbRev:  replicationState.AuthDBRev,
		ModifiedTs: replicationState.ModifiedTS.UnixMicro(),
	}
	return gs.UploadAuthDB(ctx, signedAuthDB, authDBRevision, readers)
}
