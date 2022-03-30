// Copyright 2021 The LUCI Authors.
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

// Package model contains datastore model definitions.
package model

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// AuthVersionedEntityMixin is for AuthDB entities that
// keep track of when they change.
type AuthVersionedEntityMixin struct {
	// ModifiedTS is the time this entity was last modified.
	ModifiedTS time.Time `gae:"modified_ts"`

	// ModifiedBy specifies who mode the modification to this entity.
	ModifiedBy string `gae:"modified_by"`

	// AuthDBRev is the revision number that this entity was last modified at.
	AuthDBRev int64 `gae:"auth_db_rev"`

	// AuthDBPrevRev is revision number of the previous version of this entity.
	AuthDBPrevRev int64 `gae:"auth_db_prev_rev"`
}

// AuthGlobalConfig is the root entity for auth models.
// There should be only one instance of this model in datastore.
type AuthGlobalConfig struct {
	// AuthVersionedEntityMixin is embedded
	// to include modification details related to this entity.
	AuthVersionedEntityMixin

	Kind string `gae:"$kind,AuthGlobalConfig"`
	ID   string `gae:"$id,root"`

	// OAuthClientID is an OAuth2 client_id to use to mint new OAuth2 tokens.
	OAuthClientID string `gae:"oauth_client_id,noindex"`

	// OAuthAdditionalClientIDs are the additional client ID's allowed to access
	// the service.
	OAuthAdditionalClientIDs []string `gae:"oauth_additional_client_ids,noindex"`

	// OAuthClientSecret is passed to clients.
	OAuthClientSecret string `gae:"oauth_client_secret,noindex"`

	// TokenServerURL is the URL of the token server to use to generate delegation
	// tokens.
	TokenServerURL string `gae:"token_server_url,noindex"`

	// SecurityConfig is serialized security config from security_config.proto.
	//
	// See https://source.chromium.org/chromium/infra/infra/+/main:luci/appengine/components/components/auth/proto/security_config.proto.
	SecurityConfig []byte `gae:"security_config,noindex"`
}

// AuthReplicationState contains revision number incremented each time a part
// of AuthDB changes.
type AuthReplicationState struct {
	Kind string `gae:"$kind,AuthReplicationState"`
	ID   string `gae:"$id,self"`

	// Parent is RootKey().
	Parent *datastore.Key `gae:"$parent"`

	// AuthDBRev is increased by 1 with every change to the AuthDB that should be
	// distributed to linked services.
	AuthDBRev int64 `gae:"auth_db_rev,noindex"`

	// ModifiedTS is the time this entity was last modified.
	ModifiedTS time.Time `gae:"modified_ts,noindex"`

	// Deprecated fields kept for compatibility with the Python version. They can
	// be removed once the Python version is not running any more.

	PrimaryID  string   `gae:"primary_id,noindex"`
	PrimaryURL string   `gae:"primary_url,noindex"`
	ShardIDs   []string `gae:"shard_ids,noindex"`
}

// AuthGroup is a group of identities, the entity id is the group name.
type AuthGroup struct {
	// AuthVersionedEntityMixin is embedded
	// to include modification details related to this entity.
	AuthVersionedEntityMixin

	Kind string `gae:"$kind,AuthGroup"`
	ID   string `gae:"$id"`

	// Parent is RootKey().
	Parent *datastore.Key `gae:"$parent"`

	// Members is the list of members that are explicitly in this group.
	Members []string `gae:"members"`

	// Globs is the list of identity-glob expressions in this group.
	//
	// Example: ('user@example.com').
	Globs []string `gae:"globs"`

	// Nested is the list of nested group names.
	Nested []string `gae:"nested"`

	// Description is a human readable description of this group.
	Description string `gae:"description,noindex"`

	// Owners is the name of the group that owns this group.
	// Owners can modify or delete this group.
	Owners string `gae:"owners"`

	// CreatedTS is the time when this group was created, in UTC.
	CreatedTS time.Time `gae:"created_ts"`

	// CreatedBy is the email of the user who created this group.
	CreatedBy string `gae:"created_by"`
}

type AuthIPAllowlist struct {
	// AuthVersionedEntityMixin is embedded
	// to include modification details related to this entity.
	AuthVersionedEntityMixin

	Kind string `gae:"$kind,AuthIPWhitelist"`
	ID   string `gae:"$id"`

	// Parent is RootKey().
	Parent *datastore.Key `gae:"$parent"`

	// Subnets is the list of subnets for this allowlist.
	Subnets []string `gae:"subnets"`

	// Description is a human readable description of this allowlist.
	Description string `gae:"description,noindex"`

	// CreatedTS is the time when this allowlist was created, in UTC.
	CreatedTS time.Time `gae:"created_ts"`

	// CreatedBy is the email of the user who created this allowlist.
	CreatedBy string `gae:"created_by"`
}

// AuthDBSnapshot Contains deflated serialized ReplicationPushRequest for some revision.
// Root entity. ID is corresponding revision number (as integer). Immutable.
type AuthDBSnapshot struct {
	Kind string `gae:"$kind,AuthDBSnapshot"`
	ID   int64  `gae:"$id"`

	// AuthDBDeflated is the deflated serialized ReplicationPushRequest proto message.
	AuthDBDeflated []byte `gae:"auth_db_deflated,noindex"`

	// ShardIDs is a list of shard IDs if sharded or empty if auth_db_deflated should be used.
	ShardIDs []string `gae:"shard_ids,noindex"`

	// AuthDBSha256 is the SHA256 hex digest of auth_db (before compression).
	AuthDBSha256 string `gae:"auth_db_sha256,noindex"`

	// CreatedTS is when this snapshot was created.
	CreatedTS time.Time `gae:"created_ts"`
}

// AuthDBShard is a shard of deflated serialized ReplicationPushRequest.
// Root entity. ID is "<auth_db_revision:<blob hash>
type AuthDBShard struct {
	Kind string `gae:"$kind,AuthDBShard"`
	ID   string `gae:"$id"`

	// Blob represents a sharded part of the full AuthDB blob.
	Blob []byte `gae:"blob,noindex"`
}

// AuthDBSnapshotLatest is a Pointer to latest stored AuthDBSnapshot entity.
// Exists in single instance with key ('AuthDBSnapshotLatest', 'latest').
type AuthDBSnapshotLatest struct {
	Kind string `gae:"$kind,AuthDBSnapshotLatest"`
	ID   string `gae:"$id"`

	// AuthDBRev is the revision number of latest stored AuthDBSnaphost. Monotonically increases.
	AuthDBRev int64 `gae:"auth_db_rev,noindex"`

	// AuthDBSha256 is the SHA256 hex digest of auth_db (before compression).
	AuthDBSha256 string `gae:"auth_db_sha256,noindex"`

	// CreatedTS is when this snapshot was created.
	ModifiedTS time.Time `gae:"modified_ts,noindex"`
}

// RootKey gets the root key of the entity group with all AuthDB entities.
func RootKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "AuthGlobalConfig", "root", 0, nil)
}

// GetReplicationState fetches AuthReplicationState from the datastore.
//
// Returns datastore.ErrNoSuchEntity if it is missing.
func GetReplicationState(ctx context.Context) (*AuthReplicationState, error) {
	state := &AuthReplicationState{
		Kind:   "AuthReplicationState",
		ID:     "self",
		Parent: RootKey(ctx),
	}

	switch err := datastore.Get(ctx, state); {
	case err == nil:
		return state, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthReplicationState").Err()
	}
}

// GetAuthGroup gets the AuthGroup with the given id(groupName).
//
// Returns datastore.ErrNoSuchEntity if the group given is not present.
// Returns an annotated error for other errors.
func GetAuthGroup(ctx context.Context, groupName string) (*AuthGroup, error) {
	authGroup := &AuthGroup{
		Kind:   "AuthGroup",
		ID:     groupName,
		Parent: RootKey(ctx),
	}

	switch err := datastore.Get(ctx, authGroup); {
	case err == nil:
		return authGroup, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthGroup").Err()
	}
}

// GetAllAuthGroups returs all the AuthGroups from the datastore.
//
// Returns an annotated error.
func GetAllAuthGroups(ctx context.Context) ([]*AuthGroup, error) {
	query := datastore.NewQuery("AuthGroup").Ancestor(RootKey(ctx))
	var authGroups []*AuthGroup
	err := datastore.GetAll(ctx, query, &authGroups)
	if err != nil {
		return nil, errors.Annotate(err, "error getting all AuthGroup entities").Err()
	}
	return authGroups, nil
}

// GetAuthIPAllowlist gets the AuthIPAllowlist with the given allowlist name.
//
// Returns datastore.ErrNoSuchEntity if the allowlist given is not present.
// Returns an annotated error for other errors.
func GetAuthIPAllowlist(ctx context.Context, allowlistName string) (*AuthIPAllowlist, error) {
	authIPAllowlist := &AuthIPAllowlist{
		Kind:   "AuthIPWhitelist",
		ID:     allowlistName,
		Parent: RootKey(ctx),
	}

	switch err := datastore.Get(ctx, authIPAllowlist); {
	case err == nil:
		return authIPAllowlist, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthIPAllowlist").Err()
	}
}

// GetAllAuthIPAllowlists returns all the AuthIPAllowlist entities from the
// datastore, these are stored as AuthIPWhitelist in the datastore.
//
// Returns an annotated error.
func GetAllAuthIPAllowlists(ctx context.Context) ([]*AuthIPAllowlist, error) {
	query := datastore.NewQuery("AuthIPWhitelist").Ancestor(RootKey(ctx))
	var authIPAllowlists []*AuthIPAllowlist
	err := datastore.GetAll(ctx, query, &authIPAllowlists)
	if err != nil {
		return nil, errors.Annotate(err, "error getting all AuthIPAllowlist entities").Err()
	}
	return authIPAllowlists, nil
}

// GetAuthGlobalConfig returns the AuthGlobalConfig datastore entity.
//
// Returns datastore.ErrNoSuchEntity if the AuthGlobalConfig is not present.
// Returns an annotated error for other errors.
func GetAuthGlobalConfig(ctx context.Context) (*AuthGlobalConfig, error) {
	authGlobalConfig := &AuthGlobalConfig{
		Kind: "AuthGlobalConfig",
		ID:   "root",
	}

	switch err := datastore.Get(ctx, authGlobalConfig); {
	case err == nil:
		return authGlobalConfig, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthGlobalConfig").Err()
	}
}

// GetAuthDBSnapshot returns the AuthDBSnapshot datastore entity.
//
// Returns datastore.ErrNoSuchEntity if the AuthDBSnapshot is not present.
// Returns an annotated error for other errors.
func GetAuthDBSnapshot(ctx context.Context, rev int64, skipBody bool) (*AuthDBSnapshot, error) {
	authDBSnapshot := &AuthDBSnapshot{
		Kind: "AuthDBSnapshot",
		ID:   rev,
	}
	switch err := datastore.Get(ctx, authDBSnapshot); {
	case err == nil:
		if skipBody {
			authDBSnapshot.AuthDBDeflated = nil
		} else if len(authDBSnapshot.ShardIDs) != 0 {
			authDBSnapshot.AuthDBDeflated, err = unshardAuthDB(ctx, authDBSnapshot.ShardIDs)
			if err != nil {
				return nil, err
			}
		}

		return authDBSnapshot, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthDBSnapshot %d", rev).Err()
	}
}

// GetAuthDBSnapshotLatest returns the AuthDBSnapshotLatest datastore entity.
//
// Returns datastore.ErrNoSuchEntity if the AuthDBSnapshotLatest is not present.
// Returns an annotated error for other errors.
func GetAuthDBSnapshotLatest(ctx context.Context) (*AuthDBSnapshotLatest, error) {
	authDBSnapshotLatest := &AuthDBSnapshotLatest{
		Kind: "AuthDBSnapshotLatest",
		ID:   "latest",
	}

	switch err := datastore.Get(ctx, authDBSnapshotLatest); {
	case err == nil:
		return authDBSnapshotLatest, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthDBSnapshotLatest").Err()
	}
}

// Fetches a list of AuthDBShard entities and merges their payload.
//
// shardIDs:
// 	a list of shard IDs as produced by shard_authdb.
// 	comes in format "authdb_rev:sha256hash(blob)", e.g. 42:7F404D83A3F4405...
//
// Returns datastore.ErrNoSuchEntity if an AuthDBShard is not present.
// Returns an annotated error for other errors.
// Returns the merged AuthDB blob on success.
func unshardAuthDB(ctx context.Context, shardIDs []string) ([]byte, error) {
	shards := make([]*AuthDBShard, len(shardIDs))
	for index, id := range shardIDs {
		shards[index] = &AuthDBShard{
			Kind: "AuthDBShard",
			ID:   id,
		}
	}

	if err := datastore.Get(ctx, shards); err != nil {
		return nil, errors.Annotate(err, "error getting AuthDBShards").Err()
	}

	authDBBlobSize := 0
	for _, shard := range shards {
		authDBBlobSize += len(shard.Blob)
	}

	authDBBlob := make([]byte, 0, authDBBlobSize)
	for _, shard := range shards {
		authDBBlob = append(authDBBlob, shard.Blob...)
	}
	return authDBBlob, nil
}

// ToProto converts the AuthGroup entity to the protobuffer
// equivalent.
func (group *AuthGroup) ToProto() *rpcpb.AuthGroup {
	return &rpcpb.AuthGroup{
		Name:        group.ID,
		Members:     group.Members,
		Globs:       group.Globs,
		Nested:      group.Nested,
		Description: group.Description,
		Owners:      group.Owners,
		CreatedTs:   timestamppb.New(group.CreatedTS),
		CreatedBy:   group.CreatedBy,
	}
}

// ToProto converts the AuthIPAllowlist entity to the protobuffer
// equivalent Allowlist.
func (allowlist *AuthIPAllowlist) ToProto() *rpcpb.Allowlist {
	return &rpcpb.Allowlist{
		Name:        allowlist.ID,
		Subnets:     allowlist.Subnets,
		Description: allowlist.Description,
		CreatedTs:   timestamppb.New(allowlist.CreatedTS),
		CreatedBy:   allowlist.CreatedBy,
	}
}

// ToProto converts the AuthDBSnapshot entity to the protobuffer
// equivalent Snapshot.
func (snapshot *AuthDBSnapshot) ToProto() *rpcpb.Snapshot {
	return &rpcpb.Snapshot{
		AuthDbRev:      snapshot.ID,
		AuthDbSha256:   snapshot.AuthDBSha256,
		AuthDbDeflated: snapshot.AuthDBDeflated,
		CreatedTs:      timestamppb.New(snapshot.CreatedTS),
	}
}
