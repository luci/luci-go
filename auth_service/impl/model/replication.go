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
	"encoding/base64"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
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
	logging.Infof(ctx, "enqueuing %d", authdbrev)
	return tq.AddTask(ctx, &tq.Task{
		Payload: &taskspb.ReplicationTask{AuthDbRev: authdbrev},
		Title:   fmt.Sprintf("authdb-rev-%d", authdbrev),
	})
}

func handleReplicationTask(ctx context.Context, task *taskspb.ReplicationTask, dryRun, useV1Perms bool) error {
	authDBRev := task.GetAuthDbRev()
	logging.Infof(ctx, "replicating AuthDB for Rev %d (dry run: %v)", authDBRev, dryRun)

	if err := replicate(ctx, authDBRev, dryRun, useV1Perms); err != nil {
		if !dryRun && transient.Tag.In(err) {
			// Return the error to signal retry.
			return err
		}

		// Either dryRun is enabled, or error is non-transient;
		// do not retry.
		logging.Errorf(ctx, "error replicating AuthDB: %s", err)
		return nil
	}
	return nil
}

// replicate triggers AuthDB replication for all client types.
//
// Note: to avoid stale tasks, it will first check that the
// AuthReplicationState.AuthDBRev is still equal to the
// given authDBRev before doing anthing.
func replicate(ctx context.Context, authDBRev int64, dryRun, useV1Perms bool) error {
	replicationState, err := GetReplicationState(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to get current replication state").Err()
	}

	// Check the task is not stale before doing any heavy lifting.
	if replicationState.AuthDBRev != authDBRev {
		logging.Infof(ctx, "skipping stale replication task; requested Rev %d but current Rev is already %d",
			authDBRev, replicationState.AuthDBRev)
		return nil
	}

	// Pack the entire AuthDB into a blob to be stored in the datastore,
	// Google Storage and directly pushed to Replicas.
	replicationState, revisionInfo, authDBBlob, err := packAuthDB(ctx, useV1Perms)
	if err != nil {
		return errors.Annotate(err, "failed to pack AuthDB").Err()
	}

	// Put the blob into datastore. Also updates pointer to the latest
	// stored blob. This is used by the endpoint at
	// /auth_service/api/v1/authdb/revisions/<rev|"latest">
	if err := StoreAuthDBSnapshot(ctx, replicationState, authDBBlob, dryRun); err != nil {
		return errors.Annotate(err, "failed to store AuthDBSnapshot to datastore").Err()
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
		return errors.Annotate(err, "error signing AuthDB").Err()
	}

	// Put the blob into Google Storage, if the path has been configured.
	gsPath, err := gs.GetPath(ctx)
	if err == nil && gs.IsValidPath(gsPath) {
		// Upload to GS.
		err := uploadToGS(ctx, replicationState, authDBBlob, sig, keyName, dryRun)
		if err != nil {
			logging.Errorf(ctx, "failed to upload AuthDB Rev %d to GS: %s", authDBRev, err)
			return err
		}
	}

	// Notify PubSub subscribers that a new snapshot is available.
	if err := pubsub.PublishAuthDBRevision(ctx, revisionInfo, dryRun); err != nil {
		logging.Errorf(ctx, "error publishing PubSub message for revision %d: %s",
			revisionInfo.AuthDbRev, err)
		return err
	}

	// Directly push the latest AuthDB to replicas.
	if err := updateReplicas(ctx, revisionInfo.AuthDbRev, authDBBlob, sig, keyName, dryRun); err != nil {
		logging.Errorf(ctx, "error updating replicas for revision %d: %s",
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
func packAuthDB(ctx context.Context, useV1Perms bool) (*AuthReplicationState, *protocol.AuthDBRevision, []byte, error) {
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

	authDBProto, err := snapshot.ToAuthDBProto(useV1Perms)
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

func uploadToGS(ctx context.Context, replicationState *AuthReplicationState, authDBBlob, sig []byte, keyName string, dryRun bool) error {
	readers, err := GetAuthorizedEmails(ctx)
	if err != nil {
		return errors.Annotate(err, "error getting authorized reader emails").Err()
	}

	serviceAccount, err := getServiceAccountName(ctx)
	if err != nil {
		return errors.Annotate(err, "error getting service account for signed AuthDB").Err()
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
	return gs.UploadAuthDB(ctx, signedAuthDB, authDBRevision, readers, dryRun)
}

func updateReplicas(ctx context.Context, authDBRev int64, authDBBlob, sig []byte, keyName string, dryRun bool) error {
	// Get last known replica states.
	staleReplicas, err := GetAllStaleReplicas(ctx, authDBRev)
	if err != nil {
		logging.Errorf(ctx, "error getting all stale AuthReplicaStates: %s", err)
		return err
	}

	if len(staleReplicas) == 0 {
		logging.Infof(ctx, "all replicas are up-to-date")
		return nil
	}
	logging.Debugf(ctx, "%d stale replicas need to be updated (dryRun: %v)",
		len(staleReplicas), dryRun)

	// Exit early for dry run to skip direct push to replicas.
	if dryRun {
		return nil
	}

	// Push the AuthDB to all replicas in parallel.
	encodedSig := base64.StdEncoding.EncodeToString(sig)
	err = parallel.FanOutIn(func(workC chan<- func() error) {
		for _, staleReplica := range staleReplicas {
			replicaState := staleReplica
			workC <- func() error {
				return ReplicateToReplica(ctx, replicaState, authDBRev,
					authDBBlob, keyName, encodedSig)
			}
		}
	})

	// parallel.FanOutIn returns nil if there were no errors at all,
	// or an errors.MultiError.
	if err != nil {
		// At least one replica push update returned an error. Check if at
		// least one replica returned a non-fatal (so retriable) error.
		shouldRetry := false
		if merr, ok := err.(errors.MultiError); ok {

			for _, e := range merr {
				if e != nil && !errors.Is(e, FatalReplicaUpdateError) {
					shouldRetry = true
					break
				}
			}
		}
		if shouldRetry {
			// Annotate the error with the transient tag.
			err = errors.Annotate(err,
				"replica push update needs to be retried").Tag(transient.Tag).Err()
			logging.Errorf(ctx, "returning transient error to retry replication: %s", err)
		}
		return err
	}

	return nil
}

func getServiceAccountName(ctx context.Context) (string, error) {
	signer := auth.GetSigner(ctx)
	if signer == nil {
		return "", errors.New("error getting the Signer instance for the service")
	}

	info, err := signer.ServiceInfo(ctx)
	if err != nil {
		return "", errors.Annotate(err, "failed to get service info").Err()
	}

	return info.ServiceAccountName, nil
}
