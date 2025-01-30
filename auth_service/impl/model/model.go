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
	stdzlib "compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/impl/util/zlib"
	"go.chromium.org/luci/auth_service/internal/permissions"

	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

const (
	// AdminGroup defines a group whose members are allowed to create new groups.
	AdminGroup = "administrators"

	// TrustedServicesGroup defines a group whose members are allowed to
	// subscribe to group change notifications and fetch all groups at
	// once.
	TrustedServicesGroup = "auth-trusted-services"

	// Max size of AuthDBShard.Blob.
	MaxShardSize = 900 * 1024 // 900 kB.
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

	// PermissionsList is all globally defined permissions, in alphabetical order.
	PermissionsList *permissions.PermissionsList `gae:"permissionslist"`

	// Ignore unrecognized fields and strip in future writes.
	_ datastore.PropertyMap `gae:"-,extra"`
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
	// Note: to avoid hitting the Datastore limit of the maximimum size of an
	// unindexed property, the realms are compressed before being stored.
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
	ErrInvalidReference = stderrors.New("some referenced groups don't exist")
	// ErrInvalidIdentity is returned when a referenced identity or glob is invalid.
	ErrInvalidIdentity = stderrors.New("invalid identity")
	// ErrConcurrentModification is returned when an entity is modified by two concurrent operations.
	ErrConcurrentModification = stderrors.New("concurrent modification")
	// ErrReferencedEntity is returned when an entity cannot be deleted because it is referenced elsewhere.
	ErrReferencedEntity = stderrors.New("cannot delete referenced entity")
	// ErrCyclicDependency is returned when an update would create a cyclic dependency.
	ErrCyclicDependency = stderrors.New("groups can't have cyclic dependencies")
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
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthReplicationState").Err()
	}
}

// IsExternalAuthGroupName checks if the given name is a valid external auth group
// name. This also implies that the auth group is not editable.
func IsExternalAuthGroupName(name string) bool {
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

// RealmsToProto processes the Realms field and returns it as a proto.
//
// Returns an annotated error if one occurs.
func (apr *AuthProjectRealms) RealmsToProto() (projectRealms *protocol.Realms, err error) {
	return FromStorableRealms(apr.Realms)
}

// ProjectID returns the project that an AuthProjectRealmsMeta is connected to.
func (aprm *AuthProjectRealmsMeta) ProjectID() (string, error) {
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
		if errors.Is(err, datastore.ErrNoSuchEntity) {
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
	if groupName == "" {
		return nil, fmt.Errorf("%w: empty group name", ErrInvalidName)
	}

	authGroup := makeAuthGroup(ctx, groupName)

	switch err := datastore.Get(ctx, authGroup); {
	case err == nil:
		// Set the Owners field to the admin group if it's empty, which may
		// happen if the group was created by the Python version of
		// Auth Service.
		if authGroup.Owners == "" {
			authGroup.Owners = AdminGroup
		}
		return authGroup, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthGroup").Err()
	}
}

// GetAllAuthGroups returns all the AuthGroups from the datastore.
//
// Returns an annotated error.
func GetAllAuthGroups(ctx context.Context) ([]*AuthGroup, error) {
	query := datastore.NewQuery("AuthGroup").Ancestor(RootKey(ctx))
	var authGroups []*AuthGroup
	err := datastore.GetAll(ctx, query, &authGroups)
	if err != nil {
		return nil, errors.Annotate(err, "error getting all AuthGroup entities").Err()
	}
	for _, authGroup := range authGroups {
		// Set the Owners field to the admin group if it's empty, which may
		// happen if the group was created by the Python version of
		// Auth Service.
		if authGroup.Owners == "" {
			authGroup.Owners = AdminGroup
		}
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
		return fmt.Errorf("%w: %s", ErrInvalidReference, strings.Join(missingRefs, ", "))
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
//
// Note: creating external groups using this function is forbidden, as they are
// handled by the importer. See importer.go in this package.
func CreateAuthGroup(ctx context.Context, group *AuthGroup, historicalComment string) (*AuthGroup, error) {
	// Check the supplied group name is valid, and not an external group.
	if !auth.IsValidGroupName(group.ID) || IsExternalAuthGroupName(group.ID) {
		return nil, ErrInvalidName
	}

	// Check that the supplied members and globs are well-formed.
	if err := validateIdentities(group.Members); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidIdentity, err)
	}
	if err := validateGlobs(group.Globs); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidIdentity, err)
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

		// Set the Owners to AdminGroup if not set.
		if newGroup.Owners == "" {
			newGroup.Owners = AdminGroup
		}

		// Check that all referenced groups (owning group, nested groups) exist.
		// It is ok for a new group to have itself as owner.
		// Note: we can skip checking for cycles because all existing groups
		// should be valid and thus won't have any cycles.
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
	var keys []*datastore.Key
	if err := datastore.GetAll(ctx, q, &keys); err != nil {
		return nil, err
	}
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = key.StringID()
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

// UpdateAuthGroup updates the given AuthGroup.
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
func UpdateAuthGroup(ctx context.Context, groupUpdate *AuthGroup, updateMask *fieldmaskpb.FieldMask, etag, historicalComment string) (*AuthGroup, error) {
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
	if IsExternalAuthGroupName(groupUpdate.ID) {
		return nil, errors.Annotate(ErrPermissionDenied, "cannot update external group").Err()
	}

	// Do some preliminary validation before entering the Datastore transaction.
	for _, field := range updateMask.GetPaths() {
		switch field {
		case "members":
			// Check that the supplied members are well-formed.
			if err := validateIdentities(groupUpdate.Members); err != nil {
				return nil, fmt.Errorf("%w: %s", ErrInvalidIdentity, err)
			}
		case "globs":
			// Check that the supplied globs are well-formed.
			if err := validateGlobs(groupUpdate.Globs); err != nil {
				return nil, fmt.Errorf("%w: %s", ErrInvalidIdentity, err)
			}
		case "owners":
			// Admin group must always be owned by itself.
			// Note: empty owners field is permitted as the admin group is the
			// default for owners.
			if groupUpdate.ID == AdminGroup && groupUpdate.Owners != "" && groupUpdate.Owners != AdminGroup {
				return nil, fmt.Errorf("%w: changing %q group owners is forbidden", ErrInvalidArgument, AdminGroup)
			}
		}
	}

	err := runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		var ok bool
		// Fetch the group and check the user is an admin or a group owner.
		authGroup, err := GetAuthGroup(ctx, groupUpdate.ID)
		if err != nil {
			return err
		}
		ok, err = auth.IsMember(ctx, AdminGroup, authGroup.Owners)
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
		updated := false
		for _, field := range updateMask.GetPaths() {
			switch field {
			case "members":
				if !slices.Equal(authGroup.Members, groupUpdate.Members) {
					authGroup.Members = groupUpdate.Members
					updated = true
				}
			case "globs":
				if !slices.Equal(authGroup.Globs, groupUpdate.Globs) {
					authGroup.Globs = groupUpdate.Globs
					updated = true
				}
			case "nested":
				if slices.Equal(authGroup.Nested, groupUpdate.Nested) {
					continue
				}

				// Check that any new groups being added exist.
				addingNestedGroups := stringset.NewFromSlice(groupUpdate.Nested...)
				addingNestedGroups.DelAll(authGroup.Nested)
				if err := checkGroupsExist(ctx, addingNestedGroups.ToSlice()); err != nil {
					return err
				}
				authGroup.Nested = groupUpdate.Nested
				updated = true

				// Check for group dependency cycles given the new nested groups.
				if cycle, err := findGroupDependencyCycle(ctx, authGroup); err != nil {
					return err
				} else if cycle != nil {
					cycleStr := strings.Join(cycle, " -> ")
					return fmt.Errorf("%w: %s", ErrCyclicDependency, cycleStr)
				}
			case "description":
				if authGroup.Description != groupUpdate.Description {
					authGroup.Description = groupUpdate.Description
					updated = true
				}
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
				updated = true
			default:
				return errors.Annotate(ErrInvalidArgument, "unknown field: %s", field).Err()
			}
		}

		if !updated {
			// No changes were made; nothing to do.
			logging.Infof(ctx, "skipping update of AuthGroup %q; already up to date", authGroup.ID)
			return nil
		}

		// Do additional validation if it's the admin group being updated.
		if authGroup.ID == AdminGroup {
			if err := validateAdminGroup(ctx, authGroup); err != nil {
				return err
			}
		}

		// Commit the update.
		return commitEntity(authGroup, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), false)
	})
	if err != nil {
		return nil, err
	}
	// Refetch the group as saved in datastore so subsequent updates work.
	updatedGroup, err := GetAuthGroup(ctx, groupUpdate.ID)
	if err != nil {
		return nil, errors.Annotate(err, "get group after update failed").Err()
	}
	return updatedGroup, nil
}

func validateAdminGroup(ctx context.Context, admin *AuthGroup) error {
	// The admin group must own itself.
	if admin.Owners != AdminGroup {
		return errors.Annotate(ErrInvalidArgument,
			"%s must be owned by itself", AdminGroup).Err()
	}

	// Forbid globs because the admin group is very privileged.
	if len(admin.Globs) > 0 {
		return errors.Annotate(ErrInvalidArgument,
			"%s cannot have globs", AdminGroup).Err()
	}

	// Forbid internal subgroups, as this could lead to an unexpectedly large
	// admin group over time.
	for _, nested := range admin.Nested {
		if !IsExternalAuthGroupName(nested) {
			return errors.Annotate(ErrInvalidArgument,
				"%s can only have external subgroups", AdminGroup).Err()
		}
	}
	// If here, all nested subgroups are external.

	// The admin group cannot be empty, i.e.
	// - there should be at least one explicit member, OR
	// - there should be at least one external subgroup so there is a pathway
	//   to admin membership.
	if len(admin.Members) > 0 || len(admin.Nested) > 0 {
		return nil
	}

	return errors.Annotate(ErrInvalidArgument,
		"%s cannot be empty", AdminGroup).Err()
}

// DeleteAuthGroup deletes the specified AuthGroup.
//
// Possible errors:
//
//	datastore.ErrNoSuchEntity if the specified group does not exist.
//	ErrPermissionDenied if the caller is not allowed to delete the group.
//	ErrConcurrentModification if the provided etag is not up-to-date.
//	ErrReferencedEntity if the group is referenced by another group.
//	Annotated error for other errors.
func DeleteAuthGroup(ctx context.Context, groupName string, etag string, historicalComment string) error {
	// Disallow deletion of the admin group.
	if groupName == AdminGroup {
		return ErrPermissionDenied
	}

	// External groups cannot be manually deleted.
	if IsExternalAuthGroupName(groupName) {
		return errors.Annotate(ErrPermissionDenied, "cannot delete external group").Err()
	}

	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
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
	case errors.Is(err, datastore.ErrNoSuchEntity):
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

// updateAllAuthIPAllowlists updates all the entities in datastore from the
// subnet map, which should be the result of parsing an ip_allowlist.cfg.
func updateAllAuthIPAllowlists(ctx context.Context, subnetMap map[string][]string, historicalComment string) error {
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

		serviceIdentity, err := getServiceIdentity(ctx)
		if err != nil {
			return err
		}
		now := clock.Now(ctx).UTC()

		toCreate := updatedAllowlistSet.Difference(oldAllowlistSet)
		for id := range toCreate {
			entity := makeAuthIPAllowlist(ctx, id)

			entity.Subnets = subnetMap[id]
			entity.Description = "Imported from ip_allowlist.cfg"
			entity.CreatedBy = string(serviceIdentity)
			entity.CreatedTS = now

			if err := commitEntity(entity, now, serviceIdentity, false); err != nil {
				return err
			}
		}

		toUpdate := oldAllowlistSet.Intersect(updatedAllowlistSet)
		for id := range toUpdate {
			entity := oldAllowlistMap[id]
			oldSubnets := entity.Subnets
			newSubnets := subnetMap[id]

			// Do a simple slice comparison. It is assumed the given subnetMap
			// slice values have been normalized and ordered already.
			if slices.Equal(oldSubnets, newSubnets) {
				// No value change, so skip update.
				continue
			}

			logging.Debugf(ctx,
				"Identified change in subnets for IPAllowlist %s:\n(old) %v\n(new) %v",
				id, oldSubnets, newSubnets,
			)

			entity.Subnets = newSubnets
			if err := commitEntity(entity, now, serviceIdentity, false); err != nil {
				return err
			}
		}

		toDelete := oldAllowlistSet.Difference(updatedAllowlistSet)
		for id := range toDelete {
			if err := commitEntity(oldAllowlistMap[id], now, serviceIdentity, true); err != nil {
				return err
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
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthGlobalConfig").Err()
	}
}

func hasSameOauthCfg(authGlobalConfig *AuthGlobalConfig, oauthCfg *configspb.OAuthConfig) bool {
	if oauthCfg.GetPrimaryClientId() != authGlobalConfig.OAuthClientID {
		return false
	}
	if oauthCfg.GetPrimaryClientSecret() != authGlobalConfig.OAuthClientSecret {
		return false
	}
	if oauthCfg.GetTokenServerUrl() != authGlobalConfig.TokenServerURL {
		return false
	}
	if !slices.Equal(oauthCfg.GetClientIds(), authGlobalConfig.OAuthAdditionalClientIDs) {
		return false
	}
	return true
}

func hasSameSecurityCfg(authGlobalConfig *AuthGlobalConfig, securityCfg *protocol.SecurityConfig) (bool, error) {
	storedSecurityCfg := &protocol.SecurityConfig{}
	if err := proto.Unmarshal(authGlobalConfig.SecurityConfig, storedSecurityCfg); err != nil {
		return false, errors.Annotate(err, "failed to unmarshal stored AuthGlobalConfig security config").Err()
	}

	return proto.Equal(securityCfg, storedSecurityCfg), nil
}

// updateAuthGlobalConfig updates the AuthGlobalConfig datastore entity.
// If there is no AuthGlobalConfig entity present in the datastore, one will be
// created.
func updateAuthGlobalConfig(ctx context.Context, oauthCfg *configspb.OAuthConfig, securityCfg *protocol.SecurityConfig, historicalComment string) error {
	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		shouldCreate := false

		// Get current AuthGlobalConfig entity.
		rootAuthGlobalCfg, err := GetAuthGlobalConfig(ctx)
		if err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
			return err
		}
		if rootAuthGlobalCfg == nil {
			// No previous AuthGlobalConfig - need to store a new one.
			shouldCreate = true
			rootAuthGlobalCfg = makeAuthGlobalConfig(ctx)
		}

		sameOauth := hasSameOauthCfg(rootAuthGlobalCfg, oauthCfg)
		sameSecurity, err := hasSameSecurityCfg(rootAuthGlobalCfg, securityCfg)
		if err != nil {
			return err
		}

		// Exit early if the global config exists and hasn't actually changed.
		if !shouldCreate && sameOauth && sameSecurity {
			logging.Infof(ctx, "skipping update of AuthGlobalConfig; already up to date")
			return nil
		}

		// AuthGlobalConfig needs to be updated.
		rootAuthGlobalCfg.OAuthClientID = oauthCfg.GetPrimaryClientId()
		rootAuthGlobalCfg.OAuthAdditionalClientIDs = oauthCfg.GetClientIds()
		rootAuthGlobalCfg.OAuthClientSecret = oauthCfg.GetPrimaryClientSecret()
		rootAuthGlobalCfg.TokenServerURL = oauthCfg.GetTokenServerUrl()
		securityCfgBlob, err := proto.Marshal(securityCfg)
		if err != nil {
			return err
		}
		rootAuthGlobalCfg.SecurityConfig = securityCfgBlob

		serviceIdentity, err := getServiceIdentity(ctx)
		if err != nil {
			return err
		}

		if err := commitEntity(rootAuthGlobalCfg, clock.Now(ctx).UTC(), serviceIdentity, false); err != nil {
			return err
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
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthDBSnapshot %d", rev).Err()
	}
}

// StoreAuthDBSnapshot stores the AuthDB blob (serialized proto) into
// Datastore.
//
// Args:
//   - replicationState: the AuthReplicationState corresponding to the
//     given authDBBlob.
//   - authDBBlob: serialized protocol.ReplicationPushRequest message
//     (has AuthDB inside).
func StoreAuthDBSnapshot(ctx context.Context, replicationState *AuthReplicationState, authDBBlob []byte) (err error) {
	logging.Debugf(ctx, "storing AuthDB Rev %d", replicationState.AuthDBRev)

	// Get the SHA256 hex digest of the AuthDB blob (before compression).
	blobChecksum := sha256.Sum256(authDBBlob)
	blobHexDigest := hex.EncodeToString(blobChecksum[:])

	// Get the deflated serialized protocol.ReplicationPushRequest message.
	deflated, err := zlib.Compress(authDBBlob)
	if err != nil {
		return errors.Annotate(err, "error compressing AuthDB").Err()
	}

	// Split it into shards to avoid hitting entity size limits. Do it
	// only if `deflated` is larger than the limit. Otherwise it is more
	// efficient to store it inline in AuthDBSnapshot (it is also how it
	// is stored in older entities, before sharding was introduced).
	var shardIDs []string
	if len(deflated) > MaxShardSize {
		shardIDs, err = shardAuthDB(ctx, replicationState.AuthDBRev, deflated, MaxShardSize)
		if err != nil {
			return errors.Annotate(err, "error sharding AuthDB").Err()
		}
	}

	// Attempt to add the AuthDBSnapshot to datastore.
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Check for an existing snapshot at this revision.
		authDBSnapshot := &AuthDBSnapshot{
			Kind: "AuthDBSnapshot",
			ID:   replicationState.AuthDBRev,
		}
		err := datastore.Get(ctx, authDBSnapshot)
		if err == nil {
			// Already exists.
			logging.Infof(ctx, "skipping storing of AuthDBSnapshot - already exists for Rev %d", replicationState.AuthDBRev)
			return nil
		} else if !errors.Is(err, datastore.ErrNoSuchEntity) {
			// Unexpected error when checking datastore.
			return errors.Annotate(err, "error when checking for existing AuthDBSnapshot").Err()
		}

		// AuthDBSnapshot does not exist for this revision, so it can be
		// written.
		authDBSnapshot.AuthDBSha256 = blobHexDigest
		authDBSnapshot.CreatedTS = replicationState.ModifiedTS
		// Set either AuthDBDeflated or ShardIDs.
		if len(shardIDs) > 0 {
			authDBSnapshot.ShardIDs = shardIDs
		} else {
			authDBSnapshot.AuthDBDeflated = deflated
		}
		return datastore.Put(ctx, authDBSnapshot)
	}, nil)
	if err != nil {
		return errors.Annotate(err, "error storing AuthDBSnapshot").Err()
	}

	// Update AuthDBSnapshotLatest, if necessary.
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		latest := &AuthDBSnapshotLatest{
			Kind: "AuthDBSnapshotLatest",
			ID:   "latest",
		}
		err := datastore.Get(ctx, latest)
		if err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
			return err
		}

		if latest.AuthDBRev < replicationState.AuthDBRev {
			latest.AuthDBRev = replicationState.AuthDBRev
			latest.ModifiedTS = replicationState.ModifiedTS
			latest.AuthDBSha256 = blobHexDigest
			return datastore.Put(ctx, latest)
		}

		return nil
	}, nil)
	if err != nil {
		return errors.Annotate(err, "error updating AuthDBSnapshotLatest").Err()
	}

	return nil
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
	case errors.Is(err, datastore.ErrNoSuchEntity):
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
	case errors.Is(err, datastore.ErrNoSuchEntity):
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

// updateAuthRealmsGlobals updates the AuthRealmsGlobals singleton entity in
// datastore, creating the entity if necessary.
//
// Returns an annotated error if one occurs.
func updateAuthRealmsGlobals(ctx context.Context, permsCfg *configspb.PermissionsConfig, historicalComment string) error {
	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		stored, err := GetAuthRealmsGlobals(ctx)
		if err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
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
				if cfgPermsNames.Has(name) {
					continue
				}

				cfgPermsNames.Add(name)
				cfgPermsMap[name] = p
			}
		}

		// The list of permissions must be in alphabetical order. See
		// https://pkg.go.dev/go.chromium.org/luci/server/auth/service/protocol#Realms
		orderedCfgPerms := make([]*protocol.Permission, len(cfgPermsNames))
		for i, name := range cfgPermsNames.ToSortedSlice() {
			orderedCfgPerms[i] = cfgPermsMap[name]
		}

		// Exit early if the permissions haven't actually changed.
		if stored.PermissionsList != nil && permsEqual(storedPermsList, orderedCfgPerms) {
			logging.Infof(ctx, "skipping update of AuthRealmsGlobals; stored.PermissionsList matches latest permissions.cfg")
			return nil
		}

		serviceIdentity, err := getServiceIdentity(ctx)
		if err != nil {
			return err
		}

		stored.PermissionsList = &permissions.PermissionsList{
			Permissions: orderedCfgPerms,
		}
		return commitEntity(stored, clock.Now(ctx).UTC(), serviceIdentity, false)
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
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting AuthProjectRealms %s", project).Err()
	}
}

// GetAllAuthProjectRealms returns all the AuthProjectRealms entities in datastore.
//
// Returns an annotated error.
func GetAllAuthProjectRealms(ctx context.Context) ([]*AuthProjectRealms, error) {
	query := datastore.NewQuery("AuthProjectRealms").Ancestor(RootKey(ctx))
	var authProjectRealms []*AuthProjectRealms
	err := datastore.GetAll(ctx, query, &authProjectRealms)
	if err != nil {
		return nil, errors.Annotate(err, "error getting all AuthProjectRealms entities").Err()
	}
	return authProjectRealms, nil
}

// deleteAuthProjectRealms deletes an AuthProjectRealms entity from datastore.
// The caller is expected to handle the error. Also attempts to delete the
// corresponding AuthProjectRealmsMeta, but does not return an error if deleting
// that fails.
//
// Returns error if Get from datastore failed
// Returns error if transaction delete failed
func deleteAuthProjectRealms(ctx context.Context, project string, historicalComment string) error {
	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		authProjectRealms, err := GetAuthProjectRealms(ctx, project)
		if err != nil {
			return err
		}

		serviceIdentity, err := getServiceIdentity(ctx)
		if err != nil {
			return err
		}

		// Delete the AuthProjectRealms for this project.
		if err := commitEntity(authProjectRealms, clock.Now(ctx).UTC(), serviceIdentity, true); err != nil {
			return err
		}

		// Delete the corresponding AuthProjectRealmsMeta for this project.
		if err := deleteAuthProjectRealmsMeta(ctx, project); err != nil {
			// Non-fatal - the AuthProjectRealms was successfully deleted.
			// Just log the error.
			logging.Errorf(
				ctx, "failed to delete corresponding AuthProjectRealmsMeta for project %s: %s",
				project, err)
		}
		return nil
	})
}

// deleteAuthProjectRealmsMeta deletes an AuthProjectRealmsMeta entity from
// datastore. The caller is expected to handle the error.
//
// Returns error if Get from datastore failed.
// Returns error if the delete transaction failed.
func deleteAuthProjectRealmsMeta(ctx context.Context, project string) error {
	meta, err := GetAuthProjectRealmsMeta(ctx, project)
	if err != nil {
		return err
	}

	if err := datastore.Delete(ctx, meta); err != nil {
		return errors.Annotate(err, "error deleting meta realms for project %s", project).Err()
	}

	return nil
}

// RealmsCfgRev is information about fetched or previously processed realms.cfg.
// Comes either from LUCI Config (then `ConfigBody` is set, but `PermsRev`
// isn't) or from the datastore (then `PermsRev` is set, but `ConfigBody`
// isn't). All other fields are always set.
type RealmsCfgRev struct {
	ProjectID    string
	ConfigRev    string
	ConfigDigest string

	// These two are mutually exclusive.
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

// updateAuthProjectRealms updates all the realms for a specific LUCI Project.
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
func updateAuthProjectRealms(ctx context.Context, eRealms []*ExpandedRealms, permsRev string, historicalComment string) error {
	return runAuthDBChange(ctx, historicalComment, func(ctx context.Context, commitEntity commitAuthEntity) error {
		metas := []*AuthProjectRealmsMeta{}
		existing := []*AuthProjectRealms{}
		for _, r := range eRealms {
			currentProjectRealms, err := GetAuthProjectRealms(ctx, r.CfgRev.ProjectID)
			if err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
				return err
			}
			existing = append(existing, currentProjectRealms)
		}

		serviceIdentity, err := getServiceIdentity(ctx)
		if err != nil {
			return err
		}
		now := clock.Now(ctx).UTC()

		for idx, r := range eRealms {
			realms, err := ToStorableRealms(r.Realms)
			if err != nil {
				return err
			}
			if existing[idx] == nil {
				// create new one
				newRealm := makeAuthProjectRealms(ctx, r.CfgRev.ProjectID)
				newRealm.Realms = realms
				newRealm.ConfigRev = r.CfgRev.ConfigRev
				newRealm.PermsRev = permsRev
				if err := commitEntity(newRealm, now, serviceIdentity, false); err != nil {
					return errors.Annotate(err, "failed to create new AuthProjectRealm %s", r.CfgRev.ProjectID).Err()
				}
			} else if !bytes.Equal(existing[idx].Realms, realms) {
				// update
				existing[idx].Realms = realms
				existing[idx].ConfigRev = r.CfgRev.ConfigRev
				existing[idx].PermsRev = permsRev
				if err := commitEntity(existing[idx], now, serviceIdentity, false); err != nil {
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
	case errors.Is(err, datastore.ErrNoSuchEntity):
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

// shardAuthDB splits the given blob into multiple AuthDBShard entities.
//
// Stores shards sequentially to avoid making a bunch of memory-hungry
// calls in parallel.
//
// Args:
//   - authDBRev: AuthDB revision to use in AuthDBShard entity keys.
//   - blob: the data to split into shards.
//   - maxSize: the maximum shard size.
//
// Returns:
//   - the IDs of the shards that the blob was sequentially written to.
func shardAuthDB(ctx context.Context, authDBRev int64, blob []byte, maxSize int) ([]string, error) {
	logging.Debugf(ctx, "sharding AuthDB Rev %d", authDBRev)

	var shard []byte
	shardIDs := []string{}
	for len(blob) > 0 {
		if len(blob) < maxSize {
			maxSize = len(blob)
		}

		shard, blob = blob[:maxSize], blob[maxSize:]
		shardChecksum := sha256.Sum256(shard)
		shardID := fmt.Sprintf("%d:%s", authDBRev, hex.EncodeToString(shardChecksum[:]))
		authDBShard := &AuthDBShard{
			Kind: "AuthDBShard",
			ID:   shardID,
			Blob: shard,
		}

		if err := datastore.Put(ctx, authDBShard); err != nil {
			return []string{}, err
		}
		shardIDs = append(shardIDs, shardID)
	}

	return shardIDs, nil
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
func (group *AuthGroup) ToProto(ctx context.Context, includeMemberships bool) (*rpcpb.AuthGroup, error) {
	isModifier, err := CanCallerModify(ctx, group)
	if err != nil {
		return nil, err
	}

	membersVisible, err := CanCallerViewMembers(ctx, group)
	if err != nil {
		return nil, err
	}

	authGroup := &rpcpb.AuthGroup{
		Name:                 group.ID,
		Description:          group.Description,
		Owners:               group.Owners,
		CreatedTs:            timestamppb.New(group.CreatedTS),
		CreatedBy:            group.CreatedBy,
		Etag:                 group.etag(),
		CallerCanModify:      isModifier,
		CallerCanViewMembers: membersVisible,
	}

	if includeMemberships {
		if membersVisible {
			authGroup.Members = group.Members
		} else {
			authGroup.NumRedacted = int32(len(group.Members))
		}
		authGroup.Globs = group.Globs
		authGroup.Nested = group.Nested
	}
	return authGroup, nil
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

// /////////////////////////////////////////////////////////////////////
// ///////////////////// Realms helper functions ///////////////////////
// /////////////////////////////////////////////////////////////////////

func ToStorableRealms(realms *protocol.Realms) ([]byte, error) {
	marshalled, err := proto.Marshal(realms)
	if err != nil {
		return nil, errors.Annotate(err, "error marshalling realms").Err()
	}
	compressed, err := zlib.Compress(marshalled)
	if err != nil {
		return nil, errors.Annotate(err, "error compressing realms").Err()
	}

	return compressed, nil
}

func FromStorableRealms(blob []byte) (*protocol.Realms, error) {
	marshalled, err := zlib.Decompress(blob)
	switch {
	case err == nil:
		// Do nothing; data was successfully decompressed.
	case errors.Is(err, stdzlib.ErrHeader):
		// The data didn't have a valid ZLIB header. Assume it was not
		// ZLIB-compressed when last stored.
		marshalled = blob
	default:
		return nil, errors.Annotate(err, "error decompressing realms").Err()
	}

	realms := &protocol.Realms{}
	if err := proto.Unmarshal(marshalled, realms); err != nil {
		return nil, errors.Annotate(err, "error unmarshalling realms").Err()
	}

	return realms, nil
}
