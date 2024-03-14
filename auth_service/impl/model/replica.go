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
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// The possible values for the PushStatus field of AuthReplicaState.
const (
	ReplicaPushStatusSuccess ReplicaPushStatus = iota
	ReplicaPushStatusTransientError
	ReplicaPushStatusFatalError
)

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

// getAllStaleReplicas gets all the AuthReplicaState entities that are
// behind the given authDBRev.
func getAllStaleReplicas(ctx context.Context, authDBRev int64) ([]*AuthReplicaState, error) {
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
