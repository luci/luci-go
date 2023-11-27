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
	"bytes"
	"context"
	"encoding/base64"
	stderrors "errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/service/protocol"
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

// etag returns an opaque string to use as an etag for the entity.
// In reality it just encodes the last-modified time in base64, but this shouldn't
// be relied on by callers as the implementation may change in future.
func (e *AuthVersionedEntityMixin) etag() string {
	bytes, err := e.ModifiedTS.MarshalText()
	if err != nil {
		// This is only possible if the timestamp was invalid, which should
		// never happen.
		panic("invalid timestamp passed to etagFromTimestamp")
	}
	b64 := base64.StdEncoding.EncodeToString(bytes)
	return fmt.Sprintf(`W/"%s"`, b64)
}

// makeHistoricalCopy returns a PropertyLoadSaver that represents a historical
// copy of the provided entity at its current revision, plus some metadata:
//   - whether the entity was deleted at this revision
//   - a comment describing the change that created this entity version
//   - the app version with which the change was created
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

// AuthRealmsGlobals is a singleton entity with global portions of realms configuration.
//
// Data here does not relate to any individual realm or project. Currently
// contains a list of all defined permissions (with their metadata).
type AuthRealmsGlobals struct {
	// AuthVersionedEntityMixin is embedded
	// to include modification details related to this entity.
	AuthVersionedEntityMixin

	Kind string `gae:"$kind,AuthRealmsGlobal"`
	ID   string `gae:"$id,globals"`

	Parent *datastore.Key `gae:"$parent"`

	// TODO(cjacomet): Remove this once Python version no longer depends on
	// this.
	//
	// Permissions and PermissionsList are the same, they are just represented
	// differently in datastore from Python to Golang. In Python lists of pointers
	// to protobuf types would serialize automagically and store as an array of
	// serialized strings in datastore. In Go this is not supported in the luci datastore
	// package. It is better to define the list as repeated protobuf.Msg instead of
	// []*protobuf.Msg. This will yield the same results in the end but with a bit different steps.
	// Permissions will not be unmarshalled in this version, the declaration
	// is to avoid any issues when fetching the entity from datastore.
	//
	// Permissions is all globally defined permissions, in alphabetical order.
	Permissions []string `gae:"permissions,noindex"`

	// PermissionsList is all globally defined permissions, in alphabetical order.
	PermissionsList *permissions.PermissionsList `gae:"permissionslist"`
}

// AuthProjectRealms is all realms of a single LUCI project.
//
// They are defined as Realms proto message reduced to a single project:
//   - Only project's realms are listed in `realms` field.
//   - Only permissions used by the project are listed in `permissions` field.
//   - Permissions have their metadata stripped, they have only names.
type AuthProjectRealms struct {
	// AuthVersionedEntityMixin is embedded
	// to include modification details related to this entity.
	AuthVersionedEntityMixin

	Kind string `gae:"$kind,AuthProjectRealms"`
	ID   string `gae:"$id"`

	Parent *datastore.Key `gae:"$parent"`

	// Realms is all the realms of a project, see comment for AuthProjectRealms.
	Realms []byte `gae:"realms,noindex"`

	// ConfigRev is the git revision the config was picked up from.
	ConfigRev string `gae:"config_rev,noindex"`

	// PermsRev is the revision of permissions DB used to expand roles.
	PermsRev string `gae:"perms_rev,noindex"`
}

// AuthProjectRealmsMeta is the metadata of some AuthProjectRealms entity.
//
// Always created/deleted/updated transactionally with the corresponding
// AuthProjectRealms entity, but it is not part of AuthDB itself.
//
// Used to hold bookkeeping state related to realms.cfg processing. Can
// be fetched very efficiently (compared to fetching AuthProjectRealms).
//
// ID is always meta, the parent entity is the corresponding AuthProjectRealms.
type AuthProjectRealmsMeta struct {
	Kind string `gae:"$kind,AuthProjectRealmsMeta"`
	ID   string `gae:"$id,meta"`

	Parent *datastore.Key `gae:"$parent"`

	// ConfigRev is the revision the config was picked up from.
	ConfigRev string `gae:"config_rev,noindex"`

	// PermsRev is the revision of permissions.cfg used to expand roles.
	PermsRev string `gae:"perms_rev,noindex"`

	// ConfigDigest is a SHA256 digest of the raw config body.
	ConfigDigest string `gae:"config_digest,noindex"`

	// ModifiedTS is when it was last updated, (mostly FYI).
	ModifiedTS time.Time `gae:"modified_ts,noindex"`
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
	// ErrInvalidArgument is a generic error returned when an argument is invalid.
	ErrInvalidArgument = stderrors.New("invalid argument")
	// ErrInvalidName is returned when a supplied entity name is invalid.
	ErrInvalidName = stderrors.New("invalid entity name")
	// ErrInvalidReference is returned when a referenced entity name is invalid.
	ErrInvalidReference = stderrors.New("invalid reference")
	// ErrInvalidIdentity is returned when a referenced identity or glob is invalid.
	ErrInvalidIdentity = stderrors.New("invalid identity")
	// ErrConcurrentModification is returned when an entity is modified by two concurrent operations.
	ErrConcurrentModification = stderrors.New("concurrent modification")
	// ErrReferencedEntity is returned when an entity cannot be deleted because it is referenced elsewhere.
	ErrReferencedEntity = stderrors.New("cannot delete referenced entity")
	// ErrCyclicDependency is returned when an update would create a cyclic dependency.
	ErrCyclicDependency = stderrors.New("circular dependency")
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

// PreviousHistoricalRevisionKey gets the key for the previous history entity of the given
// historical entity PropertyMap.
func PreviousHistoricalRevisionKey(ctx context.Context, pm datastore.PropertyMap) *datastore.Key {
	if getProp(pm, "auth_db_prev_rev") != nil {
		return datastore.NewKey(ctx, "Rev", "", getInt64Prop(pm, "auth_db_prev_rev"), RootKey(ctx))
	}
	return nil
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

// isExternalAuthGroupName checks if the given name is a valid external auth group
// name. This also implies that the auth group is not editable.
func isExternalAuthGroupName(name string) bool {
	return strings.Contains(name, "/") && auth.IsValidGroupName(name)
}

// makeAuthGroup is a convenience function for creating an AuthGroup with the given ID.
func makeAuthGroup(ctx context.Context, id string) *AuthGroup {
	return &AuthGroup{
		Kind:   "AuthGroup",
		ID:     id,
		Parent: RootKey(ctx),
	}
}

// makeAuthIPAllowlist is a convenience function for creating an AuthIPAllowlist with the given ID.
func makeAuthIPAllowlist(ctx context.Context, id string) *AuthIPAllowlist {
	return &AuthIPAllowlist{
		Kind:   "AuthIPWhitelist",
		ID:     id,
		Parent: RootKey(ctx),
	}
}

// makeAuthRealmsGlobals is a convenience function for creating an AuthRealmsGlobals.
func makeAuthRealmsGlobals(ctx context.Context) *AuthRealmsGlobals {
	return &AuthRealmsGlobals{
		Kind:   "AuthRealmsGlobals",
		ID:     "globals",
		Parent: RootKey(ctx),
	}
}

// makeAuthProjectRealms is a convenience function for creating an AuthProjectRealms for a given project.
func makeAuthProjectRealms(ctx context.Context, project string) *AuthProjectRealms {
	return &AuthProjectRealms{
		Kind:   "AuthProjectRealms",
		ID:     project,
		Parent: RootKey(ctx),
	}
}

// projectRealmsKey is a convenience function for getting a key for an AuthProjectRealms.
func projectRealmsKey(ctx context.Context, project string) *datastore.Key {
	return datastore.NewKey(ctx, "AuthProjectRealms", project, 0, RootKey(ctx))
}

// makeAuthProjectRealmsMeta is a convenience function for creating an AuthProjectRealms meta for a given
// project.
func makeAuthProjectRealmsMeta(ctx context.Context, project string) *AuthProjectRealmsMeta {
	return &AuthProjectRealmsMeta{
		Kind:   "AuthProjectRealmsMeta",
		ID:     "meta",
		Parent: projectRealmsKey(ctx, project),
	}
}

// projectID is for checking the project that an AuthProjectRealmsMeta is connected to.
func (aprm *AuthProjectRealmsMeta) projectID() (string, error) {
	if aprm.Parent.Kind() != "AuthProjectRealms" {
		return "", fmt.Errorf("incorrect key pattern for AuthProjectRealmsMeta %s", aprm.ID)
	}
	return aprm.Parent.StringID(), nil
}

// makeAuthGlobalConfig is a convenience function for creating AuthGlobalConfig.
func makeAuthGlobalConfig(ctx context.Context) *AuthGlobalConfig {
	return &AuthGlobalConfig{
		Kind: "AuthGlobalConfig",
		ID:   "root",
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
func runAuthDBChange(ctx context.Context, historicalComment string, f func(context.Context, commitAuthEntity) error) error {
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
		toCommit := []any{state}

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
			historicalCopy, err := makeHistoricalCopy(ctx, entity, deletion, historicalComment)
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

		// Enqueue two backend tasks, one to generate the changelog and one to replicate the updated AuthDB to clients.
		if err := EnqueueProcessChangeTask(ctx, state.AuthDBRev); err != nil {
			return err
		}
		return EnqueueReplicationTask(ctx, state.AuthDBRev)
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
		if id, err := identity.MakeIdentity(val); err != nil {
			return err
		} else if id.Kind() == identity.Project {
			// Forbid using project identities in groups.
			// They are not safe to be added to groups until all services that consume
			// AuthDB understand them. Many services do an overzealous validation of
			// AuthDB pushes and totally reject them if AuthDB contains some unrecognized
			// identity kinds.
			return errors.New(`"project:..." identities aren't allowed in groups`)
		}
	}
	return nil
}

// validateGlobs validates that all strings in a slice are parseable as valid identity globs.
func validateGlobs(globs []string) error {
	for _, val := range globs {
		if glob, err := identity.MakeGlob(val); err != nil {
			return err
		} else if glob.Kind() == identity.Project {
			// Forbid using project identities in groups.
			// They are not safe to be added to groups until all services that consume
			// AuthDB understand them. Many services do an overzealous validation of
			// AuthDB pushes and totally reject them if AuthDB contains some unrecognized
			// identity kinds.
			return errors.New(`"project:..." globs aren't allowed in groups`)
		}
	}
	return nil
}

// checkGroupsExist checks that groups with the given names exist in datastore.
// Returns ErrInvalidReference if one or more of the groups do not exist.
func checkGroupsExist(ctx context.Context, groups []string) error {
	if len(groups) == 0 {
		return nil
	}
	keys := make([]*AuthGroup, 0, len(groups))
	for _, id := range groups {
		keys = append(keys, makeAuthGroup(ctx, id))
	}
	refsExists, err := datastore.Exists(ctx, keys)
	if err != nil {
		return errors.Annotate(err, "failed to check existence of referenced groups").Err()
	}
	if !refsExists.All() {
		missingRefs := []string{}
		for i, r := range groups {
			if !refsExists.Get(0, i) {
				missingRefs = append(missingRefs, r)
			}
		}
		return errors.Annotate(ErrInvalidReference, "some referenced groups don't exist: %s", strings.Join(missingRefs, ", ")).Err()
	}
	return nil
}

// indexOf checks whether or not a group in the provided slice has the given name.
func indexOf(groups []*AuthGroup, name string) int {
	for i, g := range groups {
		if g.ID == name {
			return i
		}
	}
	return -1
}

// findGroupDependencyCycle looks for a dependency cycle between nested groups.
// Returns the first such cycle as a list of group names, or a nil slice if
// there is no such cycle.
// It does a depth-first search starting from the specified group and fetching
// all nested groups from Datastore as it goes.
func findGroupDependencyCycle(ctx context.Context, group *AuthGroup) ([]string, error) {
	// Cache of groups that have already been fetched from Datastore.
	groups := map[string]*AuthGroup{group.ID: group}
	// Set of groups that are completely explored (all subtree is traversed).
	visited := stringset.Set{}
	// Stack containing the groups currently being explored.
	// If a cycle is detected, this will contain the cycle.
	stack := []*AuthGroup{}

	var visit func(group *AuthGroup) (bool, error)
	visit = func(group *AuthGroup) (bool, error) {
		// Load nested groups into the cache if they're not already there.
		toFetch := make([]*AuthGroup, 0, len(group.Nested))
		for _, name := range group.Nested {
			if _, ok := groups[name]; !ok {
				toFetch = append(toFetch, makeAuthGroup(ctx, name))
			}
		}
		if err := datastore.Get(ctx, toFetch); err != nil {
			return false, err
		}
		for _, group := range toFetch {
			groups[group.ID] = group
		}

		// Push current group on to the stack.
		stack = append(stack, group)
		// Examine children.
		for _, nested := range group.Nested {
			nestedGroup, ok := groups[nested]
			if !ok {
				// This shouldn't happen, but don't crash just in case.
				return false, errors.New(fmt.Sprintf("group %s not found in cache", nested))
			}
			// Cross edge. Can happen in diamond-like graph, not a cycle.
			if visited.Has(nested) {
				continue
			}
			// Back edge: group references its own ancestor -> cycle.
			if i := indexOf(stack, nested); i > -1 {
				// Add the duplicate group into the stack again before we exit,
				// so it's clear what the cycle is.
				stack = append(stack, stack[i])
				return true, nil
			}
			// Explore subtree.
			cycle, err := visit(nestedGroup)
			if err != nil {
				return false, err
			}
			if cycle {
				return true, nil
			}
		}
		// Pop current group off the stack.
		stack = stack[:len(stack)-1]
		visited.Add(group.ID)

		return false, nil
	}

	cycle, err := visit(group)
	if err != nil {
		return nil, err
	}
	if cycle {
		// If visit returned true, the stack should be non-empty.
		if len(stack) == 0 {
			panic(cycle)
		}
		names := make([]string, len(stack))
		for i, g := range stack {
			names[i] = g.ID
		}
		return names, nil
	}

	return nil, nil
}

// CreateAuthGroup creates a new AuthGroup and writes it to the datastore.
// Only the following fields will be read from the input:
// ID, Description, Owners, Members, Globs, Nested.
func CreateAuthGroup(ctx context.Context, group *AuthGroup, external bool, historicalComment string) (*AuthGroup, error) {
	if external {
		if !isExternalAuthGroupName(group.ID) {
			return nil, ErrInvalidName
		}
	} else {
		// Check the supplied group name is valid, and not an external group.
		if !auth.IsValidGroupName(group.ID) || isExternalAuthGroupName(group.ID) {
			return nil, ErrInvalidName
		}
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
	err := runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
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
		if err := checkGroupsExist(ctx, toCheck); err != nil {
			return err
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

// findGroupsWithFieldEq queries Datastore for the names of all groups where the
// given field has at least one value equal to val.
func findGroupsWithFieldEq(ctx context.Context, field string, val string) ([]string, error) {
	q := datastore.NewQuery("AuthGroup").Ancestor(RootKey(ctx)).Eq(field, val)
	var groups []*AuthGroup
	if err := datastore.GetAll(ctx, q, &groups); err != nil {
		return nil, err
	}
	names := make([]string, len(groups))
	for i, group := range groups {
		names[i] = group.ID
	}
	return names, nil
}

// findReferencingGroups finds groups that reference the specified group as nested group or owner.
//
// Used to verify that group is safe to delete, i.e. no other group is depending on it.
//
// Returns a set of names of referencing groups.
// May include the original group if it referenced itself.
func findReferencingGroups(ctx context.Context, groupName string) (stringset.Set, error) {
	var nestingGroups []string
	var ownedGroups []string
	err := parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() (err error) {
			nestingGroups, err = findGroupsWithFieldEq(ctx, "nested", groupName)
			return
		}
		work <- func() (err error) {
			ownedGroups, err = findGroupsWithFieldEq(ctx, "owners", groupName)
			return
		}
	})
	if err != nil {
		return nil, errors.Annotate(err, "error fetching referencing groups").Err()
	}
	names := stringset.New(len(nestingGroups) + len(ownedGroups))
	names.AddAll(nestingGroups)
	names.AddAll(ownedGroups)
	return names, nil
}

// UpdateAuthGroup updates the given auth group.
// The ID of the AuthGroup is used to determine which group to update.
// The field mask determines which fields of the AuthGroup will be updated.
// If the field mask is nil, all fields will be updated.
//
// Possible errors:
//
//	ErrInvalidArgument if the field mask provided is invalid.
//	ErrInvalidIdentity if any identities or globs specified in the update are invalid.
//	datastore.ErrNoSuchEntity if the specified group does not exist.
//	ErrPermissionDenied if the caller is not allowed to update the group.
//	ErrConcurrentModification if the provided etag is not up-to-date.
//	Annotated error for other errors.
func UpdateAuthGroup(ctx context.Context, groupUpdate *AuthGroup, updateMask *fieldmaskpb.FieldMask, etag string, fromExternal bool, historicalComment string) (*AuthGroup, error) {
	// A nil updateMask means we should update all fields.
	// If updateable fields are added to AuthGroup in future, they need to be
	// added to the below list.
	if updateMask == nil {
		updateMask = &fieldmaskpb.FieldMask{
			Paths: []string{
				"members",
				"globs",
				"nested",
				"description",
				"owners",
			},
		}
	}

	// External groups cannot be manually updated.
	if isExternalAuthGroupName(groupUpdate.ID) && !fromExternal {
		return nil, errors.Annotate(ErrPermissionDenied, "cannot update external group").Err()
	}

	// Do some preliminary validation before entering the Datastore transaction.
	for _, field := range updateMask.GetPaths() {
		switch field {
		case "members":
			// Check that the supplied members are well-formed.
			if err := validateIdentities(groupUpdate.Members); err != nil {
				return nil, errors.Annotate(ErrInvalidIdentity, "%s", err).Err()
			}
		case "globs":
			// Check that the supplied globs are well-formed.
			if err := validateGlobs(groupUpdate.Globs); err != nil {
				return nil, errors.Annotate(ErrInvalidIdentity, "%s", err).Err()
			}
		}
	}

	var authGroup *AuthGroup
	err := runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		var err error
		var ok bool
		// Fetch the group and check the user is an admin or a group owner.
		authGroup, err = GetAuthGroup(ctx, groupUpdate.ID)
		if err != nil {
			return err
		}
		if fromExternal {
			// Permissions check happens at ingestTarball.
			ok = true
		} else {
			ok, err = auth.IsMember(ctx, AdminGroup, authGroup.Owners)
		}
		if err != nil {
			return errors.Annotate(err, "permission check failed").Err()
		}

		if !ok {
			return ErrPermissionDenied
		}

		// Verify etag (if provided) to protect against concurrent modifications.
		if etag != "" && authGroup.etag() != etag {
			return errors.Annotate(ErrConcurrentModification, "group %q was updated by someone else", authGroup.ID).Err()
		}

		// Update fields according to the mask.
		for _, field := range updateMask.GetPaths() {
			switch field {
			case "members":
				authGroup.Members = groupUpdate.Members
			case "globs":
				authGroup.Globs = groupUpdate.Globs
			case "nested":
				// Check that any new groups being added exist.
				addingNestedGroups := stringset.NewFromSlice(groupUpdate.Nested...)
				addingNestedGroups.DelAll(authGroup.Nested)
				if err := checkGroupsExist(ctx, addingNestedGroups.ToSlice()); err != nil {
					return err
				}
				authGroup.Nested = groupUpdate.Nested

				// Check for group dependency cycles given the new nested groups.
				if cycle, err := findGroupDependencyCycle(ctx, authGroup); err != nil {
					return err
				} else if cycle != nil {
					cycleStr := strings.Join(cycle, " -> ")
					return errors.Annotate(ErrCyclicDependency, "groups can not have cyclic dependencies: %s.", cycleStr).Err()
				}
			case "description":
				authGroup.Description = groupUpdate.Description
			case "owners":
				newOwners := groupUpdate.Owners
				if newOwners == authGroup.Owners {
					continue
				}
				if newOwners == "" {
					newOwners = AdminGroup
				}
				// Check the new owners group exists.
				// No need to do this if the group owns itself.
				if newOwners != authGroup.ID {
					if err := checkGroupsExist(ctx, []string{newOwners}); err != nil {
						return err
					}
				}
				authGroup.Owners = newOwners
			default:
				return errors.Annotate(ErrInvalidArgument, "unknown field: %s", field).Err()
			}
		}

		// Commit the update.
		return commitEntity(authGroup, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), false)
	})
	if err != nil {
		return nil, err
	}
	return authGroup, nil
}

// DeleteAuthGroup deletes the specified auth group.
//
// Possible errors:
//
//	datastore.ErrNoSuchEntity if the specified group does not exist.
//	ErrPermissionDenied if the caller is not allowed to delete the group.
//	ErrConcurrentModification if the provided etag is not up-to-date.
//	ErrReferencedEntity if the group is referenced by another group.
//	Annotated error for other errors.
func DeleteAuthGroup(ctx context.Context, groupName string, etag string, external bool, historicalComment string) error {
	// Disallow deletion of the admin group.
	if groupName == AdminGroup {
		return ErrPermissionDenied
	}

	// External groups cannot be manually deleted.
	if isExternalAuthGroupName(groupName) && !external {
		return errors.Annotate(ErrPermissionDenied, "cannot delete external group").Err()
	}

	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		// Fetch the group and check the user is an admin or a group owner.
		authGroup, err := GetAuthGroup(ctx, groupName)
		if err != nil {
			return err
		}
		var ok bool
		if external {
			ok = true
		} else {
			ok, err = auth.IsMember(ctx, AdminGroup, authGroup.Owners)
		}
		if err != nil {
			return errors.Annotate(err, "permission check failed").Err()
		}
		if !ok {
			return ErrPermissionDenied
		}

		// Verify etag (if provided) to protect against concurrent modifications.
		if etag != "" && authGroup.etag() != etag {
			return errors.Annotate(ErrConcurrentModification, "group %q was updated by someone else", groupName).Err()
		}

		// Check that the group is not referenced from elsewhere.
		referencingGroups, err := findReferencingGroups(ctx, groupName)
		if err != nil {
			return errors.Annotate(err, "failed to check referencing groups").Err()
		}
		// It's okay to delete a group that references itself.
		referencingGroups.Del(groupName)
		if len(referencingGroups) > 0 {
			groupsStr := strings.Join(referencingGroups.ToSortedSlice(), ", ")
			return errors.Annotate(ErrReferencedEntity, "this group is referenced by other groups: [%s]", groupsStr).Err()
		}

		// Delete the group.
		return commitEntity(authGroup, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), true)
	})
}

// GetAuthIPAllowlist gets the AuthIPAllowlist with the given allowlist name.
//
// Returns datastore.ErrNoSuchEntity if the allowlist given is not present.
// Returns an annotated error for other errors.
func GetAuthIPAllowlist(ctx context.Context, allowlistName string) (*AuthIPAllowlist, error) {
	authIPAllowlist := makeAuthIPAllowlist(ctx, allowlistName)

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

// UpdateAllowlistEntities updates all the entities in datastore from ip_allowlist.cfg.
//
// TODO(crbug/1336135): Remove dryrun checks when turning off Python Auth Service.
func UpdateAllowlistEntities(ctx context.Context, subnetMap map[string][]string, dryRun bool, historicalComment string) error {
	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		oldAllowlists, err := GetAllAuthIPAllowlists(ctx)
		if err != nil {
			return err
		}
		oldAllowlistMap := make(map[string]*AuthIPAllowlist, len(oldAllowlists))
		oldAllowlistSet := stringset.New(len(oldAllowlists))
		for _, al := range oldAllowlists {
			oldAllowlistSet.Add(al.ID)
			oldAllowlistMap[al.ID] = al
		}

		updatedAllowlistSet := stringset.New(len(subnetMap))
		for id := range subnetMap {
			updatedAllowlistSet.Add(id)
		}

		currentIdentity := auth.CurrentIdentity(ctx)
		now := clock.Now(ctx).UTC()

		toCreate := updatedAllowlistSet.Difference(oldAllowlistSet)
		for id := range toCreate {
			entity := makeAuthIPAllowlist(ctx, id)

			entity.Subnets = subnetMap[id]
			entity.Description = "Imported from ip_allowlist.cfg"

			creator := currentIdentity
			createdTS := now

			entity.CreatedBy = string(creator)
			entity.CreatedTS = createdTS

			if dryRun {
				logging.Infof(ctx, "creating:\n%+v", entity)
			} else {
				if err := commitEntity(entity, createdTS, creator, false); err != nil {
					return err
				}
			}
		}

		toUpdate := oldAllowlistSet.Intersect(updatedAllowlistSet)
		for id := range toUpdate {
			if len(oldAllowlistMap[id].Subnets) == len(subnetMap[id]) && oldAllowlistSet.HasAll(subnetMap[id]...) {
				continue
			}
			entity := oldAllowlistMap[id]
			entity.Subnets = subnetMap[id]

			if dryRun {
				logging.Infof(ctx, "updating:\n%+v", entity)
			} else {
				if err := commitEntity(entity, now, currentIdentity, false); err != nil {
					return err
				}
			}
		}

		toDelete := oldAllowlistSet.Difference(updatedAllowlistSet)
		for id := range toDelete {
			if dryRun {
				logging.Infof(ctx, "deleting:\n%v", id)
			} else {
				if err := commitEntity(oldAllowlistMap[id], now, currentIdentity, true); err != nil {
					return err
				}
			}
		}

		return nil
	})
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

// UpdateAuthGlobalConfig updates the AuthGlobalConfig datastore entity.
// If there is no AuthGlobalConfig entity present in the datastore, one will be
// created.
//
// TODO(crbug/1336135): Remove dryrun checks when turning off Python Auth Service.
func UpdateAuthGlobalConfig(ctx context.Context, oauthcfg *configspb.OAuthConfig, seccfg *protocol.SecurityConfig, dryRun bool, historicalComment string) error {
	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		rootAuthGlobalCfg, err := GetAuthGlobalConfig(ctx)
		if err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}

		if rootAuthGlobalCfg == nil {
			rootAuthGlobalCfg = makeAuthGlobalConfig(ctx)
		}
		rootAuthGlobalCfg.OAuthClientID = oauthcfg.GetPrimaryClientId()
		rootAuthGlobalCfg.OAuthAdditionalClientIDs = oauthcfg.GetClientIds()
		rootAuthGlobalCfg.OAuthClientSecret = oauthcfg.GetPrimaryClientSecret()
		rootAuthGlobalCfg.TokenServerURL = oauthcfg.GetTokenServerUrl()
		seccfgBlob, err := proto.Marshal(seccfg)
		if err != nil {
			return err
		}
		rootAuthGlobalCfg.SecurityConfig = seccfgBlob
		if dryRun {
			logging.Infof(ctx, "updating:\n%+v", rootAuthGlobalCfg)
		} else {
			if err := commitEntity(rootAuthGlobalCfg, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), false); err != nil {
				return err
			}
		}
		return nil
	})
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

// GetAuthRealmsGlobals returns the AuthRealmsGlobals singleton datastore entity.
//
// Returns datastore.ErrNoSuchEntity if the AuthRealmsGlobals entity is not present.
// Returns an annotated error for other errors.
func GetAuthRealmsGlobals(ctx context.Context) (*AuthRealmsGlobals, error) {
	authRealmsGlobals := makeAuthRealmsGlobals(ctx)
	switch err := datastore.Get(ctx, authRealmsGlobals); {
	case err == nil:
		return authRealmsGlobals, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthRealmsGlobals").Err()
	}
}

// permsEqual returns whether two slices of permissions are equal by
// value and order.
//
// e.g.
//   - permsEqual([perm1, perm2], [perm1])                     -> false
//   - permsEqual([perm1, perm2], [proto.Clone(perm1), perm2]) -> true
//   - permsEqual([perm1, perm2], [perm2, perm1])              -> false
func permsEqual(permsA, permsB []*protocol.Permission) bool {
	return slices.EqualFunc(permsA, permsB, func(a, b *protocol.Permission) bool {
		return proto.Equal(a, b)
	})
}

// UpdateAuthRealmsGlobals updates the AuthRealmsGlobals singleton entity in datastore, creating
// the entity if necessary.
//
// Returns
//
//	formatted error if stored permissions and permissions.cfg don't match
//	proto unmarshalling error if format in datastore is incorrect.
//	internal annotated error for all other
//
// NOTE:
// Since the original property that is used to store permissions was
// of type []*protocol.Permission, we have to convert twice. This is because
// slices of pointers are not supported in luci-go gae/datastore package
// and it converts to a weird format when using datastore.Get.
// As a result we will also use the dryRun flag to just log the results
// until we don't depend on it, then once we switch we'll just stop
// writing to the entity.
//
// TODO(crbug/1336137): Remove all references and checks to old permissions
// once Python version knows how to work with permissions.cfg.
func UpdateAuthRealmsGlobals(ctx context.Context, permsCfg *configspb.PermissionsConfig, dryRun bool, historicalComment string) error {
	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		stored, err := GetAuthRealmsGlobals(ctx)
		if err != nil && err != datastore.ErrNoSuchEntity {
			return errors.Annotate(err, "error while fetching AuthRealmsGlobals entity").Err()
		}

		if stored == nil {
			stored = makeAuthRealmsGlobals(ctx)
		}

		var storedPermsList []*protocol.Permission
		if stored.PermissionsList != nil {
			storedPermsList = stored.PermissionsList.GetPermissions()
		}

		// Get the permissions from the config, using the stored
		// permissions count as a size hint for initial allocation.
		storedCount := len(storedPermsList)
		cfgPermsNames := make(stringset.Set, storedCount)
		cfgPermsMap := make(map[string]*protocol.Permission, storedCount)
		for _, r := range permsCfg.GetRole() {
			for _, p := range r.GetPermissions() {
				name := p.GetName()
				cfgPermsNames.Add(name)
				cfgPermsMap[name] = p
			}
		}

		orderedCfgPerms := make([]*protocol.Permission, len(cfgPermsNames))
		for i, name := range cfgPermsNames.ToSortedSlice() {
			orderedCfgPerms[i] = cfgPermsMap[name]
		}

		// convert what is stored by Python to proto to compare to cfg we pulled
		storedPermProto := make([]*protocol.Permission, len(stored.Permissions))

		// converting again from format we got from datastore.Get
		for i, s := range stored.Permissions {
			tempProto := &protocol.Permission{}
			err := proto.Unmarshal([]byte(s), tempProto)
			if err != nil {
				return errors.Annotate(err, "error while unmarshalling stored proto").Err()
			}
			storedPermProto[i] = tempProto
		}

		// make a set of the permission names
		storedPermsSet := stringset.New(len(storedPermProto))
		for _, p := range storedPermProto {
			storedPermsSet.Add(p.GetName())
		}

		// enforce that permissions match each other
		diff := storedPermsSet.Difference(cfgPermsNames)
		if len(diff) != 0 && stored.Permissions != nil {
			return fmt.Errorf("the stored permissions and the permissions.cfg are not the same... diff: %v", diff)
		}

		// Exit early if the permissions haven't actually changed.
		if stored.PermissionsList != nil && permsEqual(storedPermsList, orderedCfgPerms) {
			logging.Infof(ctx, "skipping update of AuthRealmsGlobals; stored.PermissionsList matches latest permissions.cfg")
			return nil
		}

		stored.PermissionsList = &permissions.PermissionsList{
			Permissions: orderedCfgPerms,
		}

		if !dryRun {
			return commitEntity(stored, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), false)
		}
		logging.Infof(ctx, "(dryRun) entity: %v", stored)
		return nil
	})
}

// GetAuthProjectRealms returns the AuthProjectRealms datastore entity for a given project.
//
// Returns datastore.ErrNoSuchEntity if the AuthProjectRealms entity is not present.
// Returns an annotated error for other errors.
func GetAuthProjectRealms(ctx context.Context, project string) (*AuthProjectRealms, error) {
	authProjectRealms := makeAuthProjectRealms(ctx, project)
	switch err := datastore.Get(ctx, authProjectRealms); {
	case err == nil:
		return authProjectRealms, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthProjectRealms %s", project).Err()
	}
}

// DeleteAuthProjectRealms deletes an AuthProjectRealms entity from datastore.
// The caller is expected to handle the error.
//
// Returns error if Get from datastore failed
// Returns error if transaction delete failed
func DeleteAuthProjectRealms(ctx context.Context, project string, historicalComment string) error {
	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		authProjectRealms, err := GetAuthProjectRealms(ctx, project)
		if err != nil {
			return err
		}
		return commitEntity(authProjectRealms, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), true)
	})
}

// RealmsCfgRev is information about fetched or previously processed realms.cfg.
// Comes either from LUCI Config (then `config_body` is set, but `perms_rev`
// isn't) or from the datastore (then `perms_rev` is set, but `config_body`
// isn't). All other fields are always set.
type RealmsCfgRev struct {
	ProjectID    string
	ConfigRev    string
	ConfigDigest string

	// Thes two are mutually exclusive
	ConfigBody []byte
	PermsRev   string
}

// ExpandedRealms contains the expanded realms and information about the expanded
// realms.
type ExpandedRealms struct {
	CfgRev *RealmsCfgRev
	// Realms is the expanded form of the realms.cfg for a project
	Realms *protocol.Realms
}

// UpdateAuthProjectRealms updates all the realms for a specific LUCI Project.
// If an entity does not exist in datastore for a given AuthProjectRealm, one
// will be created.
//
// Returns
//
//	datastore.ErrNoSuchEntity -- entity does not exist
//	Annotated Error
//		Failed to create new AuthProjectRealm entity
//		Failed to update AuthProjectRealm entity
//		Failed to put AuthProjectRealmsMeta entity
func UpdateAuthProjectRealms(ctx context.Context, eRealms []*ExpandedRealms, permsRev string, historicalComment string) error {
	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		metas := []*AuthProjectRealmsMeta{}
		existing := []*AuthProjectRealms{}
		for _, r := range eRealms {
			currentProjectRealms, err := GetAuthProjectRealms(ctx, r.CfgRev.ProjectID)
			if err != nil && err != datastore.ErrNoSuchEntity {
				return err
			}
			existing = append(existing, currentProjectRealms)
		}

		now := clock.Now(ctx).UTC()
		for idx, r := range eRealms {
			realms, err := proto.Marshal(r.Realms)
			if err != nil {
				return err
			}
			if existing[idx] == nil {
				// create new one
				newRealm := makeAuthProjectRealms(ctx, r.CfgRev.ProjectID)
				newRealm.Realms = realms
				newRealm.ConfigRev = r.CfgRev.ConfigRev
				newRealm.PermsRev = permsRev
				if err := commitEntity(newRealm, now, auth.CurrentIdentity(ctx), false); err != nil {
					return errors.Annotate(err, "failed to create new AuthProjectRealm %s", r.CfgRev.ProjectID).Err()
				}
			} else if !bytes.Equal(existing[idx].Realms, realms) {
				// update
				existing[idx].Realms = realms
				existing[idx].ConfigRev = r.CfgRev.ConfigRev
				existing[idx].PermsRev = permsRev
				if err := commitEntity(existing[idx], now, auth.CurrentIdentity(ctx), false); err != nil {
					return errors.Annotate(err, "failed to update AuthProjectRealm %s", r.CfgRev.ProjectID).Err()
				}
			} else {
				logging.Infof(ctx, "configs are fresh!")
			}

			currentMeta := makeAuthProjectRealmsMeta(ctx, r.CfgRev.ProjectID)
			currentMeta.ConfigRev = r.CfgRev.ConfigRev
			currentMeta.PermsRev = permsRev
			currentMeta.ConfigDigest = r.CfgRev.ConfigDigest
			currentMeta.ModifiedTS = now
			metas = append(metas, currentMeta)
		}
		if err := datastore.Put(ctx, metas); err != nil {
			return errors.Annotate(err, "failed trying to put AuthProjectRealmsMeta").Err()
		}
		return nil
	})
}

// GetAuthProjectRealmsMeta returns the AuthProjectRealmsMeta datastore entity for a given project.
//
// Returns datastore.ErrNoSuchEntity if the AuthProjectRealmsMeta is not present.
// Returns an annotated error for other errors.
func GetAuthProjectRealmsMeta(ctx context.Context, project string) (*AuthProjectRealmsMeta, error) {
	authProjectRealmsMeta := makeAuthProjectRealmsMeta(ctx, project)
	switch err := datastore.Get(ctx, authProjectRealmsMeta); {
	case err == nil:
		return authProjectRealmsMeta, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthProjectRealmsMeta %s", project).Err()
	}
}

// GetAllAuthProjectRealmsMeta returns all the AuthProjectRealmsMeta entities in datastore.
//
// Returns an annotated error.
func GetAllAuthProjectRealmsMeta(ctx context.Context) ([]*AuthProjectRealmsMeta, error) {
	query := datastore.NewQuery("AuthProjectRealmsMeta").Ancestor(RootKey(ctx))
	var authProjectRealmsMeta []*AuthProjectRealmsMeta
	err := datastore.GetAll(ctx, query, &authProjectRealmsMeta)
	if err != nil {
		return nil, errors.Annotate(err, "error getting all AuthProjectRealmsMeta entities").Err()
	}
	return authProjectRealmsMeta, nil
}

// Fetches a list of AuthDBShard entities and merges their payload.
//
// shardIDs:
//
//	a list of shard IDs as produced by shard_authdb.
//	comes in format "authdb_rev:sha256hash(blob)", e.g. 42:7F404D83A3F4405...
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
		Etag:        group.etag(),
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
