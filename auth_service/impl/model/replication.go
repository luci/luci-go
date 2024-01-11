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
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/taskspb"
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

func handleReplicationTask(ctx context.Context, task *taskspb.ReplicationTask, dryRun bool) error {
	authDBRev := task.GetAuthDbRev()
	logging.Infof(ctx, "replicating AuthDB for Rev %d (dry run: %s)", authDBRev, dryRun)

	if err := replicate(ctx, authDBRev, dryRun); err != nil {
		if !dryRun && transient.Tag.In(err) {
			// Return the error to signal retry.
			return err
		}

		// Either dryRun is enabled, or error is non-transient;
		// do not retry.
		logging.Errorf(ctx, "error replicating AuthDB: %v", err)
		return nil
	}
	return nil
}

// replicate triggers AuthDB replication for all client types.
//
// Note: to avoid stale tasks, it will first check that the
// AuthReplicationState.AuthDBRev is still equal to the
// given authDBRev before doing anthing.
func replicate(ctx context.Context, authDBRev int64, dryRun bool) error {
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
	replicationState, authDBBlob, err := packAuthDB(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to pack AuthDB").Err()
	}

	// Exit early for dry runs.
	if dryRun {
		logging.Infof(ctx, "(dry run) replicating AuthDB Rev %d - blob size %d and last modified %s",
			replicationState.AuthDBRev, len(authDBBlob), replicationState.ModifiedTS)
		return nil
	}

	// Put the blob into datastore. Also updates pointer to the latest
	// stored blob. This is used by the endpoint at
	// /auth_service/api/v1/authdb/revisions/<rev|"latest">
	if err := StoreAuthDBSnapshot(ctx, replicationState, authDBBlob); err != nil {
		return errors.Annotate(err, "failed to store AuthDBSnapshot to datastore").Err()
	}

	// TODO(aredulla): implement remaining client types, i.e.
	// * "direct push" replication
	// * Cloud Storage replication
	// * PubSub replication

	return nil
}

// packAuthDB packs the AuthDB into a blob (serialized proto message).
//
// Returns:
//   - replicationState: the AuthReplicationState corresponding to the
//     returned authDBBlob.
//   - authDBBlob: serialized protocol.ReplicationPushRequest message
//     (has AuthDB inside).
func packAuthDB(ctx context.Context) (*AuthReplicationState, []byte, error) {
	// Get the replication state.
	replicationState, err := GetReplicationState(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Take a snapshot of the AuthDB.
	snapshot, err := TakeSnapshot(ctx)
	if err != nil {
		return nil, nil, err
	}

	authDBProto, err := snapshot.ToAuthDBProto()
	if err != nil {
		return nil, nil, err
	}

	// Serialize to binary proto message.
	req := &protocol.ReplicationPushRequest{
		Revision: &protocol.AuthDBRevision{
			PrimaryId:  info.AppID(ctx),
			AuthDbRev:  replicationState.AuthDBRev,
			ModifiedTs: replicationState.ModifiedTS.UnixMicro(),
		},
		AuthDb:          authDBProto,
		AuthCodeVersion: AuthAPIVersion,
	}
	blob, err := proto.Marshal(req)
	if err != nil {
		return nil, nil, err
	}

	return replicationState, blob, nil
}
