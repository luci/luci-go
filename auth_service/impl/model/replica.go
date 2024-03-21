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
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"
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
