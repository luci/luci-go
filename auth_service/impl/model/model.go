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
	stderrors "errors"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	_ "go.chromium.org/luci/server/tq/txn/datastore"
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

// recordChange records a change to the versioned entity, updating its last-modified metadata.
func (e *AuthVersionedEntityMixin) recordChange(modifiedBy string, modifiedTS time.Time, authDBRev int64) {
	e.ModifiedTS = modifiedTS
	e.ModifiedBy = modifiedBy
	e.AuthDBPrevRev = e.AuthDBRev
	e.AuthDBRev = authDBRev
}

func (e *AuthVersionedEntityMixin) versionInfo() *AuthVersionedEntityMixin {
	return e
}

// makeHistoricalCopy returns a PropertyLoadSaver that represents a historical
// copy of the provided entity at its current revision, plus some metadata:
//   * whether the entity was deleted at this revision
//   * a comment describing the change that created this entity version
//   * the app version with which the change was created
//
// The copy of the entity has all of the same fields as the original, except
// the kind is modified to end in History (e.g. AuthGroup becomes AuthGroupHistory),
// the parent key is modified to include the AuthDB revision number, and all
// fields have indexes removed (they're not needed for this use case).
func makeHistoricalCopy(ctx context.Context, entity versionedEntity, deleted bool, comment string) (datastore.PropertyLoadSaver, error) {
	// Fetch all properties from the entity itself.
	pls := datastore.GetPLS(entity)

	pmap, err := pls.Save(true)
	if err != nil {
		return nil, errors.New("failed to save PropertyMap")
	}

	// Alter the Parent meta property so it specifies the current AuthDB revision.
	pmap["$parent"] = datastore.MkPropertyNI(HistoricalRevisionKey(ctx, entity.versionInfo().AuthDBRev))

	// Alter the Kind meta property to append the "History" suffix.
	kind, ok := pls.GetMeta("kind")
	if !ok {
		return nil, errors.New("failed to get $kind when creating historical copy")
	}
	pmap["$kind"] = datastore.MkPropertyNI(kind.(string) + "History")

	// Remove unset fields.
	for key, prop := range pmap {
		if prop.IsZero() {
			delete(pmap, key)
			continue
		}
	}

	// Remove indexing for all fields.
	pmap.TurnOffIdx()

	// Add historical metadata.
	pmap["auth_db_deleted"] = datastore.MkPropertyNI(deleted)
	pmap["auth_db_change_comment"] = datastore.MkPropertyNI(comment)
	pmap["auth_db_app_version"] = datastore.MkPropertyNI(info.ImageVersion(ctx))

	return pmap, nil
}

// versionedEntity is an internal interface implemented by any struct
// embedding AuthVersionedEntityMixin. It provides a convenience method for
// callers to update all of the version information on a versioned entity at
// once.
type versionedEntity interface {
	// recordChange records a change to the versioned entity, updating its
	// last-modified metadata.
	recordChange(modifiedBy string, modifiedTS time.Time, authDBRev int64)

	// versionInfo returns a pointer to the AuthVersionedEntityMixin embedded
	// by the implementing struct.
	versionInfo() *AuthVersionedEntityMixin
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

// AdminGroup defines a group whose members are allowed to create new groups.
const AdminGroup = "administrators"

var (
	// ErrAlreadyExists is returned when an entity already exists.
	ErrAlreadyExists = stderrors.New("entity already exists")
	// ErrPermissionDenied is returned when the user does not have permission for
	// the requested entity operation.
	ErrPermissionDenied = stderrors.New("permission denied")
	// ErrInvalidName is returned when a supplied entity name is invalid.
	ErrInvalidName = stderrors.New("invalid entity name")
	// ErrInvalidReference is returned when a referenced entity name is invalid.
	ErrInvalidReference = stderrors.New("invalid reference")
	// ErrInvalidIdentity is returned when a referenced identity or glob is invalid.
	ErrInvalidIdentity = stderrors.New("invalid identity")
)

// RootKey gets the root key of the entity group with all AuthDB entities.
func RootKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "AuthGlobalConfig", "root", 0, nil)
}

// HistoricalRevisionKey gets the key for an entity subgroup that holds changes
// done in a concrete revision.
func HistoricalRevisionKey(ctx context.Context, authDBRev int64) *datastore.Key {
	return datastore.NewKey(ctx, "Rev", "", authDBRev, RootKey(ctx))
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

var groupName = regexp.MustCompile(`^([a-z\-]+/)?[0-9a-z_\-\.@]{1,100}$`)

// isValidAuthGroupName checks if the given name is a valid auth group name.
func isValidAuthGroupName(name string) bool {
	return groupName.MatchString(name)
}

// isExternalAuthGroupName checks if the given name is a valid external auth group
// name. This also implies that the auth group is not editable.
func isExternalAuthGroupName(name string) bool {
	return strings.Contains(name, "/") && isValidAuthGroupName(name)
}

// makeAuthGroup is a convenience function for creating an AuthGroup with the given ID.
func makeAuthGroup(ctx context.Context, id string) *AuthGroup {
	return &AuthGroup{
		Kind:   "AuthGroup",
		ID:     id,
		Parent: RootKey(ctx),
	}
}

// commitAuthEntity is a callback function signature that allows business logic
// to commit changes to a versioned entity. It should be called by anything that
// updates the AuthDB via the runAuthDBChange wrapper function.
type commitAuthEntity func(e versionedEntity, time time.Time, author identity.Identity, deletion bool) error

// runAuthDBChange runs the provided function inside a Datastore transaction
// and provides a function that allows the caller to commit changes to entities,
// writing them to the datastore and automatically taking care of history tracking
// and incrementing the AuthDB revision.
//
// NOTE: The provided `commitAuthEntity` callback is not safe to call concurrently.
func runAuthDBChange(ctx context.Context, f func(context.Context, commitAuthEntity) error) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Get current AuthDB state and increment the revision.
		state, err := GetReplicationState(ctx)
		if err == datastore.ErrNoSuchEntity {
			// This is the first entry into the AuthDB, so create a new state with rev 0.
			state = &AuthReplicationState{
				Kind:      "AuthReplicationState",
				ID:        "self",
				Parent:    RootKey(ctx),
				AuthDBRev: 0,
			}
		} else if err != nil {
			return err
		}
		nextAuthDBRev := state.AuthDBRev + 1
		toCommit := []interface{}{state}

		// Set up a function that callers can use to commit a change to an entity.
		// If this doesn't get called, the AuthDB revision won't get incremented.
		// This should be called instead of datastore.{Put,Delete}(entity).
		commitEntity := func(entity versionedEntity, t time.Time, author identity.Identity, deletion bool) error {
			// Record change on the entity itself, and add it to the list of things
			// to be committed to Datastore.
			entity.recordChange(string(author), t, nextAuthDBRev)
			// If deleting, just delete it right now.
			// If updating, add it to the list of things to Put to the datastore.
			if deletion {
				if err := datastore.Delete(ctx, entity); err != nil {
					return err
				}
			} else {
				toCommit = append(toCommit, entity)
			}

			// Record change on the AuthDB state.
			state.AuthDBRev = nextAuthDBRev
			state.ModifiedTS = t

			// Make a historical copy of the entity.
			historicalCopy, err := makeHistoricalCopy(ctx, entity, deletion, "Go pRPC API")
			if err != nil {
				return err
			}
			toCommit = append(toCommit, historicalCopy)
			return nil
		}

		// Run the wrapped function.
		if err := f(ctx, commitEntity); err != nil {
			return err
		}

		// If the wrapped function didn't actually record an entity change, exit early.
		if state.AuthDBRev != nextAuthDBRev {
			return nil
		}

		// Commit the updated replication state and any newly created historical entities.
		if err := datastore.Put(ctx, toCommit); err != nil {
			return err
		}

		// Enqueue a backend task to process the AuthDB change.
		return EnqueueProcessChangeTask(ctx, state.AuthDBRev)
	}, nil)
}

// GetAuthGroup gets the AuthGroup with the given id(groupName).
//
// Returns datastore.ErrNoSuchEntity if the group given is not present.
// Returns an annotated error for other errors.
func GetAuthGroup(ctx context.Context, groupName string) (*AuthGroup, error) {
	authGroup := makeAuthGroup(ctx, groupName)

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

// validateIdentities validates that all strings in a slice are parseable as valid identities.
func validateIdentities(ids []string) error {
	for _, val := range ids {
		if _, err := identity.MakeIdentity(val); err != nil {
			return err
		}
	}
	return nil
}

// validateGlobs validates that all strings in a slice are parseable as valid identity globs.
func validateGlobs(globs []string) error {
	for _, val := range globs {
		if _, err := identity.MakeGlob(val); err != nil {
			return err
		}
	}
	return nil
}

// CreateAuthGroup creates a new AuthGroup and writes it to the datastore.
// Only the following fields will be read from the input:
// ID, Description, Owners, Members, Globs, Nested.
func CreateAuthGroup(ctx context.Context, group *AuthGroup) (*AuthGroup, error) {
	// Check the supplied group name is valid, and not an external group.
	if !isValidAuthGroupName(group.ID) || isExternalAuthGroupName(group.ID) {
		return nil, ErrInvalidName
	}

	// Check that the supplied members and globs are well-formed.
	if err := validateIdentities(group.Members); err != nil {
		return nil, errors.Annotate(ErrInvalidIdentity, "%s", err).Err()
	}
	if err := validateGlobs(group.Globs); err != nil {
		return nil, errors.Annotate(ErrInvalidIdentity, "%s", err).Err()
	}

	// Construct a new group so that we don't modify the input.
	newGroup := makeAuthGroup(ctx, group.ID)

	// The rest of the validation checks interact with the datastore, so run inside a transaction.
	err := runAuthDBChange(ctx, func(ctx context.Context, commitEntity commitAuthEntity) error {
		// Check the group doesn't already exist.
		exists, err := datastore.Exists(ctx, newGroup)
		if err != nil {
			return errors.Annotate(err, "failed to check whether group name already exists").Err()
		}
		if exists.Get(0) {
			return ErrAlreadyExists
		}

		// Copy in all user-settable fields.
		newGroup.Description = group.Description
		newGroup.Owners = group.Owners
		newGroup.Members = group.Members
		newGroup.Globs = group.Globs
		newGroup.Nested = group.Nested

		// Check that all referenced groups (owning group, nested groups) exist.
		// It is ok for a new group to have itself as owner.
		if newGroup.Owners == "" {
			newGroup.Owners = AdminGroup
		}
		toCheck := newGroup.Nested
		if newGroup.Owners != newGroup.ID {
			toCheck = append(toCheck, newGroup.Owners)
		}
		if len(toCheck) > 0 {
			keys := make([]*AuthGroup, 0, len(toCheck))
			for _, id := range toCheck {
				keys = append(keys, makeAuthGroup(ctx, id))
			}
			refsExists, err := datastore.Exists(ctx, keys)
			if err != nil {
				return errors.Annotate(err, "failed to check existence of referenced groups").Err()
			}
			if !refsExists.All() {
				missingRefs := []string{}
				for i, r := range toCheck {
					if !refsExists.Get(0, i) {
						missingRefs = append(missingRefs, r)
					}
				}
				return errors.Annotate(ErrInvalidReference, "some referenced groups don't exist: %s", strings.Join(missingRefs, ", ")).Err()
			}
		}

		creator := auth.CurrentIdentity(ctx)
		createdTS := clock.Now(ctx).UTC()
		newGroup.CreatedTS = createdTS
		newGroup.CreatedBy = string(creator)

		// Commit the group. This adds last-modified data on the group,
		// increments the AuthDB revision, automatically creates a historical
		// version, and triggers replication.
		return commitEntity(newGroup, createdTS, creator, false)
	})
	if err != nil {
		return nil, err
	}
	return newGroup, nil
}

// DeleteAuthGroup deletes the specified auth group.
//
// Returns datastore.ErrNoSuchEntity if the specified group does not exist.
// Returns ErrPermissionDenied if the caller is not allowed to delete the group.
// Returns an annotated error for other errors.
func DeleteAuthGroup(ctx context.Context, groupName string) error {
	// Disallow deletion of the admin group.
	if groupName == AdminGroup {
		return ErrPermissionDenied
	}

	return runAuthDBChange(ctx, func(ctx context.Context, commitEntity commitAuthEntity) error {
		// Fetch the group and check the user is an admin or a group owner.
		authGroup, err := GetAuthGroup(ctx, groupName)
		if err != nil {
			return err
		}
		ok, err := auth.IsMember(ctx, AdminGroup, authGroup.Owners)
		if err != nil {
			return errors.Annotate(err, "permission check failed").Err()
		}
		if !ok {
			return ErrPermissionDenied
		}

		// TODO(jsca): Add etag verification here to protect against concurrent modifications.

		// TODO(jsca): Check that the group is not referenced from elsewhere.

		// Delete the group.
		return commitEntity(authGroup, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), true)
	})
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

// ToProto converts the AuthGroup entity to the protobuffer equivalent.
//
// Set includeMemberships to true if we also want to include the group members, or false
// if we only want the metadata (e.g. if we're just listing groups).
func (group *AuthGroup) ToProto(includeMemberships bool) *rpcpb.AuthGroup {
	authGroup := &rpcpb.AuthGroup{
		Name:        group.ID,
		Description: group.Description,
		Owners:      group.Owners,
		CreatedTs:   timestamppb.New(group.CreatedTS),
		CreatedBy:   group.CreatedBy,
	}
	if includeMemberships {
		authGroup.Members = group.Members
		authGroup.Globs = group.Globs
		authGroup.Nested = group.Nested
	}
	return authGroup
}

// AuthGroupFromProto allocates a new AuthGroup using the values in the supplied protobuffer.
func AuthGroupFromProto(ctx context.Context, grouppb *rpcpb.AuthGroup) *AuthGroup {
	return &AuthGroup{
		Kind:        "AuthGroup",
		ID:          grouppb.GetName(),
		Parent:      RootKey(ctx),
		Members:     grouppb.GetMembers(),
		Globs:       grouppb.GetGlobs(),
		Nested:      grouppb.GetNested(),
		Description: grouppb.GetDescription(),
		Owners:      grouppb.GetOwners(),
		CreatedTS:   grouppb.GetCreatedTs().AsTime(),
		CreatedBy:   grouppb.GetCreatedBy(),
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
