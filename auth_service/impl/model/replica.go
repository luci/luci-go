// Copyright 2024 The LUCI Authors.
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

// Package model contains model definitions for Auth Service.
//
// This file contains functionality related to the "direct push" method
// of AuthDB replication.
package model

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/internal/replicas"
)

// The possible values for the PushStatus field of AuthReplicaState.
const (
	ReplicaPushStatusSuccess ReplicaPushStatus = iota
	ReplicaPushStatusTransientError
	ReplicaPushStatusFatalError
)

var (
	FatalReplicaUpdateError = errors.New("fatal replica update error")
)

// ReplicaUpdateError wraps an error that occurred when attempting to
// directly push a new AuthDB revision to a replica.
type ReplicaUpdateError struct {
	RootErr error
	IsFatal bool
}

func (r *ReplicaUpdateError) Error() string {
	return r.RootErr.Error()
}

func (r *ReplicaUpdateError) Unwrap() error {
	return r.RootErr
}

func (r *ReplicaUpdateError) Is(target error) bool {
	if target == FatalReplicaUpdateError {
		return r.IsFatal
	}
	return false
}

// ReplicaPushStatus is an enum for the replica push attempt status.
type ReplicaPushStatus int

// AuthReplicaState is the last known state of a Replica as known by
// Auth Service.
type AuthReplicaState struct {
	Kind string `gae:"$kind,AuthReplicaState"`
	// ID is GAE Application ID of the Replica.
	ID string `gae:"$id,self"`
	// Parent is replicasRootKey().
	Parent *datastore.Key `gae:"$parent"`

	// URL of a host to push updates to.
	ReplicaURL string `gae:"replica_url,noindex"`
	//Revision of auth DB replica is synced to.
	AuthDBRev int64 `gae:"auth_db_rev,noindex"`
	// Time when AuthDBRev was created (by primary clock).
	RevModifiedTS time.Time `gae:"rev_modified_ts,noindex"`
	// Value of components.auth.version.__version__ used by replica.
	AuthCodeVersion string `gae:"auth_code_version,noindex"`

	// Timestamp of when last push attempt started.
	PushStartedTS time.Time `gae:"push_started_ts,noindex"`
	// Timestamp of when last push attempt finished (successfully or not).
	PushFinishedTS time.Time `gae:"push_finished_ts,noindex"`
	// Status of last push attempt. See ReplicaPushStatus* enumeration.
	PushStatus ReplicaPushStatus `gae:"push_status,noindex"`
	// Error message of last push attempt, or empty string if it was successful.
	PushError string `gae:"push_error,noindex"`
}

// replicasRootKey is th eroot key for AuthReplicaState entities. The
// entity itself doesn't exist.
func replicasRootKey(ctx context.Context) *datastore.Key {
	// This is intentionally not under model.go's RootKey(). It has
	// nothing to do with the AuthDB itself.
	return datastore.NewKey(ctx, "AuthReplicaStateRoot", "root", 0, nil)
}

// replicaStateKey returns the corresponding key for the given app's
// AuthReplicaState entity.
func replicaStateKey(ctx context.Context, appID string) *datastore.Key {
	return datastore.NewKey(ctx, "AuthReplicaState", appID, 0, replicasRootKey(ctx))
}

// GetAllStaleReplicas gets all the AuthReplicaState entities that are
// behind the given authDBRev.
func GetAllStaleReplicas(ctx context.Context, authDBRev int64) ([]*AuthReplicaState, error) {
	query := datastore.NewQuery("AuthReplicaState").Ancestor(replicasRootKey(ctx))
	var replicaStates []*AuthReplicaState
	err := datastore.GetAll(ctx, query, &replicaStates)
	if err != nil {
		return nil, errors.Annotate(err, "error getting all AuthReplicaState entities").Err()
	}

	staleReplicas := []*AuthReplicaState{}
	for _, replica := range replicaStates {
		if replica.AuthDBRev < authDBRev {
			staleReplicas = append(staleReplicas, replica)
		}
	}

	return staleReplicas, nil
}

// ReplicateToReplica pushes a revision of the AuthDB to the given
// replica, then updates the last known state of the replica.
//
// Returns an error if:
// - the push to the replica cannot be considered successful; or
// - there was an error updating the last known state of the replica.
func ReplicateToReplica(ctx context.Context, r *AuthReplicaState,
	authDBRev int64, authDBBlob []byte, keyName, encodedSig string) error {
	if r == nil {
		return nil
	}

	started := clock.Now(ctx)
	pushResponse, pushErr := pushToReplica(ctx, r.ReplicaURL, authDBBlob,
		keyName, encodedSig)
	finished := clock.Now(ctx)

	// Eagerly update the known replica state as soon as the response is
	// received, for either failed or successful update.
	var storedRev int64
	var dsErr error
	if pushErr != nil {
		logging.Errorf(ctx, "error pushing update to replica %s: %s",
			r.ID, pushErr)
		storedRev, dsErr = updateReplicaStateOnFail(ctx, r.ID, r.AuthDBRev,
			started, finished, pushErr)
	} else {
		storedRev, dsErr = updateReplicaStateOnSuccess(ctx, r.ID,
			started, finished, pushResponse.CurrentRevision,
			pushResponse.AuthCodeVersion)
	}
	if dsErr != nil {
		if errors.Is(dsErr, datastore.ErrNoSuchEntity) {
			// The replica was removed from the known replicas, so we can
			// consider this push successful.
			logging.Debugf(ctx, "replica %s was removed", r.ID)
			return nil
		}

		// There was an issue updating the last known state for the replica.
		logging.Errorf(ctx,
			"error updating AuthReplicaState for replica %s: %s",
			r.ID, dsErr)
		return &ReplicaUpdateError{
			RootErr: dsErr,
			IsFatal: false,
		}
	}

	if pushErr != nil {
		if storedRev > authDBRev {
			// The current push failed, but some other concurrent push must
			// have succeeded (because the last known state for the replica is
			// more up-to-date than this push was attempting). Thus, we can
			// consider this push successful too.
			return nil
		}

		return pushErr
	}

	logging.Infof(ctx, "replica %s has been updated to AuthDB Rev %d",
		r.ID, storedRev)
	return nil
}

func getAuthReplicaState(ctx context.Context, key *datastore.Key) (*AuthReplicaState, error) {
	replicaState := &AuthReplicaState{}
	if populated := datastore.PopulateKey(replicaState, key); !populated {
		return nil, fmt.Errorf("failed getting AuthReplicaState; problem setting key")
	}

	if err := datastore.Get(ctx, replicaState); err != nil {
		return nil, err
	}

	return replicaState, nil
}

// updateReplicaStateOnSuccess updates an AuthReplicaState after a
// successful direct push of the AuthDB.
//
// Returns the replica's AuthDB revision as stored in Datastore after
// any updates.
func updateReplicaStateOnSuccess(ctx context.Context, replicaID string,
	started, finished time.Time, currentRevision *protocol.AuthDBRevision,
	authCodeVersion string) (int64, error) {
	var storedReplicaRev int64
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Get currently stored replica state. May be ahead of the state
		// initially fetched when processing replication tasks. If missing,
		// the replica was removed from the replication list (and shouldn't
		// be added back).
		replica, err := getAuthReplicaState(ctx, replicaStateKey(ctx, replicaID))
		if err != nil {
			return err
		}

		storedReplicaRev = replica.AuthDBRev
		if storedReplicaRev >= currentRevision.AuthDbRev {
			// The replica state has already been updated by another task;
			// don't mess with it.
			return nil
		}

		// Update the AuthReplicaState, including advancing the last known
		// revision of the AuthDB for the replica.
		replica.AuthDBRev = currentRevision.AuthDbRev
		replica.RevModifiedTS = time.UnixMicro(currentRevision.ModifiedTs).UTC()
		replica.AuthCodeVersion = authCodeVersion
		replica.PushStartedTS = started
		replica.PushFinishedTS = finished
		replica.PushStatus = ReplicaPushStatusSuccess
		replica.PushError = ""

		if err := datastore.Put(ctx, replica); err != nil {
			return err
		}
		storedReplicaRev = replica.AuthDBRev

		return nil
	}, nil)

	// Return the stored replica revision, even if there was an error
	// updating the replica state in Datastore.
	return storedReplicaRev, err
}

// updateReplicaStateOnFail updates an AuthReplicaState after a failed
// attempt to push the AuthDB to a replica.
//
// Returns the replica's AuthDB revision as stored in Datastore after
// any updates.
func updateReplicaStateOnFail(ctx context.Context, replicaID string,
	oldReplicaRev int64, started, finished time.Time, pushErr error) (int64, error) {
	var storedReplicaRev int64
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Get currently stored replica state.
		replica, err := getAuthReplicaState(ctx, replicaStateKey(ctx, replicaID))
		if err != nil {
			return err
		}

		storedReplicaRev = replica.AuthDBRev
		if storedReplicaRev > oldReplicaRev {
			// The replica state has already been modified by another task,
			// which must have successfully updated the replica as the AuthDB
			// revision is greater. So, don't mess with it.
			return nil
		}

		// Update the push attempt fields to the last known state.
		// Note: this does not advance the AuthDB revision for the replica,
		// as the update failed.
		replica.PushStartedTS = started
		replica.PushFinishedTS = finished
		replica.PushError = pushErr.Error()
		if errors.Is(pushErr, FatalReplicaUpdateError) {
			replica.PushStatus = ReplicaPushStatusFatalError
		} else {
			replica.PushStatus = ReplicaPushStatusTransientError
		}

		if err := datastore.Put(ctx, replica); err != nil {
			return err
		}

		return nil
	}, nil)

	// Return the stored replica revision, even if there was an error
	// updating the replica state in Datastore.
	return storedReplicaRev, err
}

// pushToReplica is a helper function to send the AuthDB to a replica.
// It wraps replicas.SendAuthDB, then handles processing the response
// so an appropriate ReplicaUpdateError is formed, if necessary.
func pushToReplica(ctx context.Context, replicaURL string, authDBBlob []byte,
	keyName, encodedSig string) (*protocol.ReplicationPushResponse, error) {
	res, err := replicas.SendAuthDB(ctx, replicaURL, keyName, encodedSig, authDBBlob)
	if err != nil {
		return nil, &ReplicaUpdateError{
			RootErr: errors.Annotate(err, "error sending AuthDB").Err(),
			IsFatal: true,
		}
	}
	defer func() { _ = res.Body.Close() }()

	// Any transport-level error is transient.
	if res.StatusCode != 200 {
		return nil, &ReplicaUpdateError{
			RootErr: fmt.Errorf("push request failed with HTTP code %d", res.StatusCode),
			IsFatal: false,
		}
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, &ReplicaUpdateError{
			RootErr: errors.Annotate(err, "failed to read response body").Err(),
			IsFatal: true,
		}
	}

	// Deserialize the response.
	pushResponse := &protocol.ReplicationPushResponse{}
	if err := proto.Unmarshal(body, pushResponse); err != nil {
		return nil, &ReplicaUpdateError{
			RootErr: errors.Annotate(err, "failed to unmarshal response").Err(),
			IsFatal: true,
		}
	}

	switch pushResponse.Status {
	case protocol.ReplicationPushResponse_APPLIED:
		// Do nothing; the update was successful.
	case protocol.ReplicationPushResponse_SKIPPED:
		// Do nothing; the replica had this (or a later) revision already.
	case protocol.ReplicationPushResponse_FATAL_ERROR:
		return nil, &ReplicaUpdateError{
			RootErr: fmt.Errorf("replica returned fatal error (error code %d)",
				pushResponse.ErrorCode),
			IsFatal: true,
		}
	case protocol.ReplicationPushResponse_TRANSIENT_ERROR:
		return nil, &ReplicaUpdateError{
			RootErr: fmt.Errorf("replica returned transient error (error code %d)",
				pushResponse.ErrorCode),
			IsFatal: false,
		}
	default:
		return nil, &ReplicaUpdateError{
			RootErr: fmt.Errorf("unexpected response status: %d (error code %d)",
				pushResponse.Status, pushResponse.ErrorCode),
			IsFatal: true,
		}
	}

	// The replica either applied or skipped the update; CurrentRevision
	// should be included in the response.
	if pushResponse.CurrentRevision == nil {
		return nil, &ReplicaUpdateError{
			RootErr: fmt.Errorf("incomplete response; missing CurrentRevision"),
			IsFatal: true,
		}
	}

	return pushResponse, nil
}
