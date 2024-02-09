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

package model

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/api/taskspb"
	"go.chromium.org/luci/auth_service/impl/info"
)

// ChangeType is the enum for AuthDBChange.ChangeType.
type ChangeType int64

const (
	// AuthDBGroupChange change types.
	ChangeGroupCreated            ChangeType = 1000
	ChangeGroupDescriptionChanged ChangeType = 1100
	ChangeGroupOwnersChanged      ChangeType = 1150
	ChangeGroupMembersAdded       ChangeType = 1200
	ChangeGroupMembersRemoved     ChangeType = 1300
	ChangeGroupGlobsAdded         ChangeType = 1400
	ChangeGroupGlobsRemoved       ChangeType = 1500
	ChangeGroupNestedAdded        ChangeType = 1600
	ChangeGroupNestedRemoved      ChangeType = 1700
	ChangeGroupDeleted            ChangeType = 1800

	// AuthDBIPAllowlistChange change types.
	ChangeIPALCreated            ChangeType = 3000
	ChangeIPALDescriptionChanged ChangeType = 3100
	ChangeIPALSubnetsAdded       ChangeType = 3200
	ChangeIPALSubnetsRemoved     ChangeType = 3300
	ChangeIPALDeleted            ChangeType = 3400

	// AuthDBIPAllowlistAssignmentChange change types.
	ChangeIPALAssignSet   ChangeType = 5000
	ChangeIPALAssignUnset ChangeType = 5100

	// AuthDBConfigChange change types.
	ChangeConfOauthClientChanged    ChangeType = 7000
	ChangeConfClientIDsAdded        ChangeType = 7100
	ChangeConfClientIDsRemoved      ChangeType = 7200
	ChangeConfTokenServerURLChanged ChangeType = 7300
	ChangeConfSecurityConfigChanged ChangeType = 7400

	// AuthRealmsGlobalsChange change types.
	ChangeRealmsGlobalsChanged ChangeType = 9000

	// AuthProjectRealmsChange change types.
	ChangeProjectRealmsCreated     ChangeType = 10000
	ChangeProjectRealmsChanged     ChangeType = 10100
	ChangeProjectRealmsReevaluated ChangeType = 10200
	ChangeProjectRealmsRemoved     ChangeType = 10300
)

var (
	targetRegexp = regexp.MustCompile(`^[0-9a-zA-Z_]{1,40}\$` + // entity kind
		`[0-9a-zA-Z_\-\./ @]{1,300}` + // entity ID (group, IP allowlist)
		`(\$[0-9a-zA-Z_@\-\./\:\* ]{1,200})?$`) // optional subentity ID

	changeTypeStringMap = map[ChangeType]string{
		// AuthDBGroupChange change types.
		1000: "GROUP_CREATED",
		1100: "GROUP_DESCRIPTION_CHANGED",
		1150: "GROUP_OWNERS_CHANGED",
		1200: "GROUP_MEMBERS_ADDED",
		1300: "GROUP_MEMBERS_REMOVED",
		1400: "GROUP_GLOBS_ADDED",
		1500: "GROUP_GLOBS_REMOVED",
		1600: "GROUP_NESTED_ADDED",
		1700: "GROUP_NESTED_REMOVED",
		1800: "GROUP_DELETED",

		// AuthDBIPWhitelistChange change types.
		3000: "IPWL_CREATED",
		3100: "IPWL_DESCRIPTION_CHANGED",
		3200: "IPWL_SUBNETS_ADDED",
		3300: "IPWL_SUBNETS_REMOVED",
		3400: "IPWL_DELETED",

		// AuthDBIPWhitelistAssignmentChange change types.
		5000: "IPWLASSIGN_SET",
		5100: "IPWLASSIGN_UNSET",

		// AuthDBConfigChange change types.
		7000: "CONF_OAUTH_CLIENT_CHANGED",
		7100: "CONF_CLIENT_IDS_ADDED",
		7200: "CONF_CLIENT_IDS_REMOVED",
		7300: "CONF_TOKEN_SERVER_URL_CHANGED",
		7400: "CONF_SECURITY_CONFIG_CHANGED",

		// AuthRealmsGlobalsChange change types.
		9000: "REALMS_GLOBALS_CHANGED",

		// AuthProjectRealmsChange change types.
		10000: "PROJECT_REALMS_CREATED",
		10100: "PROJECT_REALMS_CHANGED",
		10200: "PROJECT_REALMS_REEVALUATED",
		10300: "PROJECT_REALMS_REMOVED",
	}
)

// AuthDBLogRev is used to record that the changelog was generated for
// an AuthDB revision.
type AuthDBLogRev struct {
	Kind   string         `gae:"$kind,AuthDBLogRev"`
	ID     int64          `gae:"$id"` // The AuthDB revision.
	Parent *datastore.Key `gae:"$parent"`

	When       time.Time `gae:"when" json:"when"`               // When the changes were processed.
	AppVersion string    `gae:"app_version" json:"app_version"` // GAE application version that processed the change.
}

// AuthDBChange is the base (embedded) struct for change log entries.
// Has a change type and a bunch of common change properties (like who and when
// made the change). Change type order is important, it is used in UI when
// sorting changes introduces by single AuthDB commit.
//
// Change types represent minimal indivisible changes. Large AuthDB change is
// usually represented by (unordered) set of indivisible changes. For example,
// an act of creation of a new AuthDB group produces following changes:
//
//	ChangeGroupCreated
//	ChangeGroupMembersAdded
//	ChangeGroupGlobsAdded
//	ChangeGroupNestedAdded
//
// They are unordered, but UI sorts them based on change type integer. Thus
// ChangeGroupCreated appears on top: it's represented by the smallest integer.
//
// Entity id has following format:
//
//	<original_entity_kind>$<original_id>[$<subentity_id>]!<change_type>
//
// where:
//
//	original_entity_kind: a kind of modified AuthDB entity (e.g. 'AuthGroup')
//	original_id: ID of modified AuthDB entity (e.g. 'Group name')
//	subentity_id: optional identified of modified part of the entity, used for
//	  IP allowlist assignments entity (since it's just one big singleton).
//	change_type: integer Change* (see below), e.g. '1100'.
//
// Such key structure makes 'diff_entity_by_key' operation idempotent. A hash of
// entity body could have been used too, but having readable (and sortable) keys
// are nice.
//
// Parent entity is AuthDBRevision(ChangeLogRevisionKey).
//
// Note: '$' and '!' are not likely to appear in entity names since they are
// forbidden in AuthDB names. Code here also asserts this.
type AuthDBChange struct {
	Kind   string         `gae:"$kind,AuthDBChange"`
	ID     string         `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`

	Class []string `gae:"class"` // A list of "class" names giving the NDB Polymodel hierarchy

	// Fields common across all change types.
	ChangeType     ChangeType `gae:"change_type" json:"change_type"` // What kind of a change this is (see Change*)
	Target         string     `gae:"target" json:"target"`           // Entity (or subentity) that was changed: kind$id[$subid] (subid is optional).
	AuthDBRev      int64      `gae:"auth_db_rev" json:"auth_db_rev"` // AuthDB revision at which the change was made.
	Who            string     `gae:"who" json:"who"`                 // Who made the change.
	When           time.Time  `gae:"when" json:"when"`               // When the change was made.
	Comment        string     `gae:"comment" json:"comment"`         // Comment passed to record_revision or record_deletion.
	AppVersion     string     `gae:"app_version" json:"app_version"` // GAE application version at which the change was made.
	Description    string     `gae:"description,noindex" json:"description"`
	OldDescription string     `gae:"old_description,noindex" json:"old_description"`

	// Fields specific to AuthDBGroupChange.
	Owners    string   `gae:"owners" json:"owners"`         // Valid for ChangeGroupCreated and ChangeGroupOwnersChanged.
	OldOwners string   `gae:"old_owners" json:"old_owners"` // Valid for ChangeGroupOwnersChanged and ChangeGroupDeleted.
	Members   []string `gae:"members" json:"members"`       // Valid for ChangeGroupMembersAdded and ChangeGroupMembersRemoved.
	Globs     []string `gae:"globs" json:"globs"`           // Valid for ChangeGroupGlobsAdded and ChangeGroupGlobsRemoved.
	Nested    []string `gae:"nested" json:"nested"`         // Valid for ChangeGroupNestedAdded and ChangeGroupNestedRemoved.

	// Fields specific to AuthDBIPAllowlistChange.
	Subnets []string `gae:"subnets" json:"subnets"` // Valid for ChangeIPWLSubnetsAdded and ChangeIPWLSubnetsRemoved.

	// Fields specific to AuthDBIPAllowlistAssignmentChange.
	Identity    string `gae:"identity"` // Valid for ChangeIPWLAssignSet and ChangeIPWLAssignUnset.
	IPAllowlist string `gae:"ip_whitelist"`

	// Fields specific to AuthDBConfigChange.
	OauthClientID            string   `gae:"oauth_client_id" json:"oauth_client_id"`                         // Valid for ChangeConfOauthClientChanged.
	OauthClientSecret        string   `gae:"oauth_client_secret" json:"oauth_client_secret"`                 // Valid for ChangeConfOauthClientChanged.
	OauthAdditionalClientIDs []string `gae:"oauth_additional_client_ids" json:"oauth_additional_client_ids"` // Valid for ChangeConfClientIDsAdded and ChangeConfClientIDsRemoved.
	TokenServerURLOld        string   `gae:"token_server_url_old" json:"token_server_url_old"`               // Valid for ChangeConfTokenServerURLChanged.
	TokenServerURLNew        string   `gae:"token_server_url_new" json:"token_server_url_new"`               // Valid for ChangeConfTokenServerURLChanged.
	SecurityConfigOld        []byte   `gae:"security_config_old,noindex" json:"security_config_old"`         // Valid for ChangeConfSecurityConfigChanged.
	SecurityConfigNew        []byte   `gae:"security_config_new,noindex" json:"security_config_new"`         // Valid for ChangeConfSecurityConfigChanged.

	// Fields specific to AuthRealmsGlobalsChange.
	PermissionsAdded   []string `gae:"permissions_added" json:"permissions_added"`
	PermissionsChanged []string `gae:"permissions_changed" json:"permissions_changed"`
	PermissionsRemoved []string `gae:"permissions_removed" json:"permissions_removed"`

	// Fields specific to AuthProjectRealmsChange.
	ConfigRevOld string `gae:"config_rev_old,noindex"`
	ConfigRevNew string `gae:"config_rev_new,noindex"`
	PermsRevOld  string `gae:"perms_rev_old,noindex"`
	PermsRevNew  string `gae:"perms_rev_new,noindex"`
}

// ChangeLogRootKey returns the root key of an entity group with change log.
func ChangeLogRootKey(ctx context.Context, dryRun bool) *datastore.Key {
	return datastore.NewKey(ctx, entityKind("AuthDBLog", dryRun), "v1", 0, nil)
}

// ChangeLogRevisionKey returns the key of entity subgroup that keeps AuthDB
// change log for a revision.
func ChangeLogRevisionKey(ctx context.Context, authDBRev int64, dryRun bool) *datastore.Key {
	return datastore.NewKey(ctx, entityKind("AuthDBLogRev", dryRun), "", authDBRev, ChangeLogRootKey(ctx, dryRun))
}

func constructLogRevisionKey(ctx context.Context, authDBRev int64, dryRun bool) *datastore.Key {
	if authDBRev != 0 {
		return ChangeLogRevisionKey(ctx, authDBRev, dryRun)
	}

	return ChangeLogRootKey(ctx, dryRun)
}

// ChangeID returns the ID of an AuthDBChange entity based on its properties.
//
// Returns error when change.Target is invalid (doesn't contain '$' or contains '!').
func ChangeID(ctx context.Context, change *AuthDBChange) (string, error) {
	if !strings.Contains(change.Target, "$") {
		return "", errors.Reason("AuthDBChange.target %s should contain '$'", change.Target).Err()
	}
	if strings.Contains(change.Target, "!") {
		return "", errors.Reason("AuthDBChange.target %s shouldn't contain '!'", change.Target).Err()
	}
	return fmt.Sprintf("%s!%d", change.Target, change.ChangeType), nil
}

// GetAllAuthDBChange returns all the AuthDBChange entities with given target
// and authDBRev. If target is an empty string/authDBRev equals to 0, no
// target/authDBRev is specified.
//
// Returns an annotated error.
func GetAllAuthDBChange(ctx context.Context, target string, authDBRev int64, pageSize int32, pageToken string) (changes []*AuthDBChange, nextPageToken string, err error) {
	ancestor := constructLogRevisionKey(ctx, authDBRev, false)
	logging.Infof(ctx, "Ancestor: %v", ancestor)

	query := datastore.NewQuery("AuthDBChange").Ancestor(ancestor).Order("-__key__")
	if target != "" {
		if !targetRegexp.MatchString(target) {
			return nil, "", errors.Reason("Invalid target %s", target).Err()
		}
		query = query.Eq("target", target)
	}

	var cursor datastore.Cursor
	if pageToken != "" {
		cursor, err = datastore.DecodeCursor(ctx, pageToken)
		if err != nil {
			return nil, "", errors.Annotate(err, "error decoding cursor from pageToken").Err()
		}
	}
	if cursor != nil {
		query = query.Start(cursor)
	}

	var nextCur datastore.Cursor
	err = datastore.Run(ctx, query, func(change *AuthDBChange, cb datastore.CursorCB) error {
		changes = append(changes, change)
		if len(changes) >= int(pageSize) {
			if nextCur, err = cb(); err != nil {
				return err
			}
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		return nil, "", errors.Annotate(err, "error getting all AuthDBChange entities").Err()
	}
	if nextCur != nil {
		nextPageToken = nextCur.String()
	}
	return
}

func (ct ChangeType) ToString() string {
	val, ok := changeTypeStringMap[ct]
	if !ok {
		return "UNDEFINED_CHANGE_TYPE"
	}
	return val
}

func (change *AuthDBChange) ToProto() *rpcpb.AuthDBChange {
	return &rpcpb.AuthDBChange{
		ChangeType:               change.ChangeType.ToString(),
		Target:                   change.Target,
		AuthDbRev:                change.AuthDBRev,
		Who:                      change.Who,
		When:                     timestamppb.New(change.When),
		Comment:                  change.Comment,
		AppVersion:               change.AppVersion,
		Description:              change.Description,
		OldDescription:           change.OldDescription,
		Owners:                   change.Owners,
		OldOwners:                change.OldOwners,
		Members:                  change.Members,
		Globs:                    change.Globs,
		Nested:                   change.Nested,
		Subnets:                  change.Subnets,
		Identity:                 change.Identity,
		IpAllowList:              change.IPAllowlist,
		OauthClientId:            change.OauthClientID,
		OauthClientSecret:        change.OauthClientSecret,
		OauthAdditionalClientIds: change.OauthAdditionalClientIDs,
		TokenServerUrlOld:        change.TokenServerURLOld,
		TokenServerUrlNew:        change.TokenServerURLNew,
		SecurityConfigOld:        string(change.SecurityConfigOld),
		SecurityConfigNew:        string(change.SecurityConfigNew),
		PermissionsAdded:         change.PermissionsAdded,
		PermissionsChanged:       change.PermissionsChanged,
		PermissionsRemoved:       change.PermissionsRemoved,
		ConfigRevOld:             change.ConfigRevOld,
		ConfigRevNew:             change.ConfigRevNew,
		PermsRevOld:              change.PermsRevOld,
		PermsRevNew:              change.PermsRevNew,
	}
}

// EnqueueProcessChangeTask adds a ProcessChangeTask task to the cloud task
// queue.
func EnqueueProcessChangeTask(ctx context.Context, authdbrev int64) error {
	if authdbrev < 0 {
		return errors.New("negative revision numbers are not allowed")
	}
	logging.Infof(ctx, "enqueuing %d", authdbrev)
	return tq.AddTask(ctx, &tq.Task{
		Payload: &taskspb.ProcessChangeTask{AuthDbRev: authdbrev},
		Title:   fmt.Sprintf("authdb-rev-%d", authdbrev),
	})
}

func handleProcessChangeTask(ctx context.Context, task *taskspb.ProcessChangeTask, dryRun bool) error {
	authDBRev := task.GetAuthDbRev()
	logging.Infof(ctx, "processing changes for AuthDB rev %d (dry run: %v)", authDBRev, dryRun)

	_, err := generateChanges(ctx, authDBRev, dryRun)
	if err != nil {
		if !dryRun && transient.Tag.In(err) {
			// Return the error to signal retry.
			return err
		}

		// Either dryRun is enabled, or error is non-transient;
		// do not retry.
		logging.Errorf(ctx, "error generating changes: %v", err)
		return nil
	}

	return nil
}

var knownHistoricalEntities = map[string]diffFunc{
	"AuthGroupHistory":       diffGroups,
	"AuthIPWhitelistHistory": diffIPAllowlists,
	// TODO(cjacomet): AuthIPWhitelistAssignments hasn't been used since 2015,
	// either implement it in full or remove it from Python code base.
	"AuthGlobalConfigHistory":  diffGlobalConfig,
	"AuthRealmsGlobalsHistory": diffRealmsGlobals,
}

type diffFunc = func(context.Context, string, datastore.PropertyMap, datastore.PropertyMap) ([]*AuthDBChange, error)

// getAuthDBLogRev returns the AuthDBLogRev in the datastore for the
// given AuthDB revision.
//
// Note: if the AuthDBLogRev is not found, this will return (nil, nil).
func getAuthDBLogRev(ctx context.Context, authDBRev int64, dryRun bool) (*AuthDBLogRev, error) {
	logRev := &AuthDBLogRev{}
	if populated := datastore.PopulateKey(logRev, ChangeLogRevisionKey(ctx, authDBRev, dryRun)); !populated {
		return nil, errors.New("failed getting AuthDBLogRev; problem setting key")
	}

	switch err := datastore.Get(ctx, logRev); {
	case err == nil:
		return logRev, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, nil
	default:
		return nil, errors.Annotate(err, "failed getting AuthDBLogRev").Err()
	}
}

// generateChanges generates the changelog for the given AuthDB
// revision, and records it in the datastore. Returns the generated
// AuthDBChange's.
//
// Note: if the changelog for the AuthDB revision already existed, and
// thus no AuthDBChange's were added to the datastore, this will return
// (nil, nil).
func generateChanges(ctx context.Context, authDBRev int64, dryRun bool) ([]*AuthDBChange, error) {
	existingLogRev, err := getAuthDBLogRev(ctx, authDBRev, dryRun)
	if err != nil {
		return nil, err
	}
	if existingLogRev != nil {
		logging.Infof(ctx,
			"Rev %d was already processed at %s by app ver %s",
			existingLogRev.ID, existingLogRev.When, existingLogRev.AppVersion)
		return nil, nil
	}

	// If here, changelog has not been generated for this revision yet.
	query := datastore.NewQuery("").Ancestor(HistoricalRevisionKey(ctx, authDBRev))
	changes := []*AuthDBChange{}

	getPms := func(key *datastore.Key, class string, pm datastore.PropertyMap) (string, datastore.PropertyMap, datastore.PropertyMap, error) {
		target := fmt.Sprintf("%s$%s", class, key.StringID())
		var oldpm datastore.PropertyMap
		if pk := PreviousHistoricalRevisionKey(ctx, pm); pk != nil {
			oldpm = make(datastore.PropertyMap)
			oldpm.SetMeta("key", datastore.NewKey(ctx, class+"History", key.StringID(), 0, pk))
			if err := datastore.Get(ctx, oldpm); err != nil {
				return "", nil, nil, err
			}
		}
		return target, oldpm, pm, nil
	}

	err = datastore.Run(ctx, query, func(pm datastore.PropertyMap) error {
		key := getDatastoreKey(pm)
		if key == nil {
			return errors.New("key not found for pm")
		}

		re := regexp.MustCompile("((Auth(Group|IPWhitelist|IPWhitelistAssignments|GlobalConfig|RealmsGlobals|ProjectRealms))History)")
		heKeys := re.FindStringSubmatch(key.String())
		if len(heKeys) != 4 {
			return errors.New("entity not found in key")
		}

		//  match map will look like, (Entity name will change)
		// [0] = AuthGroupHistory -- Full Match
		// [1] = AuthGroupHistory -- first group
		// [2] = AuthGroup -- second group
		// [3] = Group -- third group
		if df, ok := knownHistoricalEntities[heKeys[0]]; ok {
			target, oldpm, pm, err := getPms(key, heKeys[2], pm)
			if err != nil {
				return err
			}

			diffChanges, err := df(ctx, target, oldpm, pm)
			if err != nil {
				return err
			}

			// Set the fields common to all of the changes for this entity.
			common := &AuthDBChange{
				Kind:       entityKind("AuthDBChange", dryRun),
				Parent:     constructLogRevisionKey(ctx, authDBRev, dryRun),
				AuthDBRev:  getInt64Prop(pm, "auth_db_rev"),
				Who:        getStringProp(pm, "modified_by"),
				When:       getTimeProp(pm, "modified_ts"),
				Comment:    getStringProp(pm, "auth_db_change_comment"),
				AppVersion: getStringProp(pm, "auth_db_app_version"),
			}
			for _, c := range diffChanges {
				c.Kind = common.Kind
				c.Parent = common.Parent
				c.AuthDBRev = common.AuthDBRev
				c.Who = common.Who
				c.When = common.When
				c.Comment = common.Comment
				c.AppVersion = common.AppVersion
			}

			changes = append(changes, diffChanges...)
		} else {
			return fmt.Errorf("history entity not supported %s", key.String())
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Check if the changelog was processed concurrently.
		existingLogRev, err := getAuthDBLogRev(ctx, authDBRev, dryRun)
		if err != nil {
			return err
		}
		if existingLogRev != nil {
			logging.Warningf(ctx,
				"Rev %d was already processed concurrently at %s by app ver %s",
				existingLogRev.ID, existingLogRev.When, existingLogRev.AppVersion)
			return nil
		}

		// Save the changelog and mark it as processed.
		logRev := &AuthDBLogRev{
			Kind:       entityKind("AuthDBLogRev", dryRun),
			ID:         authDBRev,
			Parent:     ChangeLogRootKey(ctx, dryRun),
			When:       clock.Now(ctx).UTC(),
			AppVersion: info.ImageVersion(ctx),
		}
		if err := datastore.Put(ctx, changes, logRev); err != nil {
			return err
		}

		if dryRun {
			// In dry run mode, skip attempting to trigger changelog
			// generation for previous revision.
			return nil
		}

		// Enqueue a task to process previous revision if not yet done.
		if authDBRev <= 1 {
			// No need to generate a changelog for the initial revision.
			return nil
		}
		prevAuthDBRev := authDBRev - 1
		existingPrevLogRev, err := getAuthDBLogRev(ctx, prevAuthDBRev, dryRun)
		if err != nil {
			// Non-fatal, just log the error; this enqueuing is for redundancy.
			logging.Errorf(
				ctx, "failed when checking changelog processed for previous AuthDB revision %d: %s",
				prevAuthDBRev, err)
		} else if existingPrevLogRev == nil {
			// Previous revision's changelog has not been processed yet.
			err = EnqueueProcessChangeTask(ctx, prevAuthDBRev)
			// Non-fatal, just log the error; this enqueuing is for redundancy.
			logging.Warningf(
				ctx, "failed when enqueuing changelog task for previous AuthDB revision %d: %s",
				prevAuthDBRev, err)
		}

		return nil
	}, nil)

	if err != nil {
		return nil, err
	}

	return changes, nil
}

func diffLists(old, new []string) ([]string, []string) {
	oldss := stringset.NewFromSlice(old...)
	newss := stringset.NewFromSlice(new...)
	return newss.Difference(oldss).ToSortedSlice(), oldss.Difference(newss).ToSortedSlice()
}

// setTargetTypeFields sets the target and change-type-related fields
// in the given AuthDBChange, and returns it for convenience.
func setTargetTypeFields(ctx context.Context, ct ChangeType, target, class string, a *AuthDBChange) *AuthDBChange {
	a.ID = fmt.Sprintf("%s!%d", target, ct)
	a.Class = []string{"AuthDBChange", class}
	a.ChangeType = ct
	a.Target = target

	return a
}

func diffGroups(ctx context.Context, target string, old, new datastore.PropertyMap) ([]*AuthDBChange, error) {
	changes := []*AuthDBChange{}
	class := "AuthDBGroupChange"

	if getBoolProp(new, "auth_db_deleted") {
		if mems := getStringSliceProp(new, "members"); mems != nil {
			changes = append(changes, setTargetTypeFields(ctx, ChangeGroupMembersRemoved, target, class, &AuthDBChange{Members: mems}))
		}
		if globs := getStringSliceProp(new, "globs"); globs != nil {
			changes = append(changes, setTargetTypeFields(ctx, ChangeGroupGlobsRemoved, target, class, &AuthDBChange{Globs: globs}))
		}
		if nested := getStringSliceProp(new, "nested"); nested != nil {
			changes = append(changes, setTargetTypeFields(ctx, ChangeGroupNestedRemoved, target, class, &AuthDBChange{Nested: nested}))
		}
		desc := getDescription(new)
		owners := AdminGroup
		if getProp(new, "owners") != nil {
			owners = getStringProp(new, "owners")
		}
		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupDeleted, target, class, &AuthDBChange{OldDescription: desc, OldOwners: owners}))
		return changes, nil
	}

	if old == nil {
		desc := getDescription(new)
		owners := AdminGroup
		if getProp(new, "owners") != nil {
			owners = getStringProp(new, "owners")
		}

		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupCreated, target, class, &AuthDBChange{Description: desc, Owners: owners}))
		if mems := getStringSliceProp(new, "members"); mems != nil {
			changes = append(changes, setTargetTypeFields(ctx, ChangeGroupMembersAdded, target, class, &AuthDBChange{Members: mems}))
		}
		if globs := getStringSliceProp(new, "globs"); globs != nil {
			changes = append(changes, setTargetTypeFields(ctx, ChangeGroupGlobsAdded, target, class, &AuthDBChange{Globs: globs}))
		}
		if nested := getStringSliceProp(new, "nested"); nested != nil {
			changes = append(changes, setTargetTypeFields(ctx, ChangeGroupNestedAdded, target, class, &AuthDBChange{Nested: nested}))
		}
		return changes, nil
	}

	if oldDesc, newDesc := getDescription(old), getDescription(new); oldDesc != newDesc {
		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupDescriptionChanged, target, class, &AuthDBChange{Description: newDesc, OldDescription: oldDesc}))
	}

	oldOwners := AdminGroup
	if getProp(old, "owners") != nil {
		oldOwners = getStringProp(old, "owners")
	}
	newOwners := getStringProp(new, "owners")
	if oldOwners != newOwners {
		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupOwnersChanged, target, class, &AuthDBChange{Owners: newOwners, OldOwners: oldOwners}))
	}

	added, removed := diffLists(getStringSliceProp(old, "members"), getStringSliceProp(new, "members"))
	if len(added) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupMembersAdded, target, class, &AuthDBChange{Members: added}))
	}
	if len(removed) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupMembersRemoved, target, class, &AuthDBChange{Members: removed}))
	}

	added, removed = diffLists(getStringSliceProp(old, "globs"), getStringSliceProp(new, "globs"))
	if len(added) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupGlobsAdded, target, class, &AuthDBChange{Globs: added}))
	}
	if len(removed) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupGlobsRemoved, target, class, &AuthDBChange{Globs: removed}))
	}

	added, removed = diffLists(getStringSliceProp(old, "nested"), getStringSliceProp(new, "nested"))
	if len(added) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupNestedAdded, target, class, &AuthDBChange{Nested: added}))
	}
	if len(removed) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeGroupNestedRemoved, target, class, &AuthDBChange{Nested: removed}))
	}

	return changes, nil
}

func diffIPAllowlists(ctx context.Context, target string, old, new datastore.PropertyMap) ([]*AuthDBChange, error) {
	changes := []*AuthDBChange{}
	class := "AuthDBIPWhitelistChange"
	if getBoolProp(new, "auth_db_deleted") {
		d := getDescription(new)
		changes = append(changes, setTargetTypeFields(ctx, ChangeIPALDeleted, target, class, &AuthDBChange{OldDescription: d}))
		if subnets := getStringSliceProp(new, "subnets"); subnets != nil {
			changes = append(changes, setTargetTypeFields(ctx, ChangeIPALSubnetsRemoved, target, class, &AuthDBChange{Subnets: subnets}))
		}
		return changes, nil
	}

	if old == nil {
		d := getDescription(new)
		changes = append(changes, setTargetTypeFields(ctx, ChangeIPALCreated, target, class, &AuthDBChange{Description: d}))
		if subnets := getStringSliceProp(new, "subnets"); subnets != nil {
			changes = append(changes, setTargetTypeFields(ctx, ChangeIPALSubnetsAdded, target, class, &AuthDBChange{Subnets: subnets}))
		}
		return changes, nil
	}

	if getDescription(old) != getDescription(new) {
		changes = append(changes, setTargetTypeFields(ctx, ChangeIPALDescriptionChanged, target, class, &AuthDBChange{Description: getDescription(new), OldDescription: getDescription(old)}))
	}

	added, removed := diffLists(getStringSliceProp(old, "subnets"), getStringSliceProp(new, "subnets"))
	if len(added) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeIPALSubnetsAdded, target, class, &AuthDBChange{Subnets: added}))
	}
	if len(removed) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeIPALSubnetsRemoved, target, class, &AuthDBChange{Subnets: removed}))
	}
	return changes, nil
}

func diffGlobalConfig(ctx context.Context, target string, old, new datastore.PropertyMap) ([]*AuthDBChange, error) {
	changes := []*AuthDBChange{}
	class := "AuthGlobalConfigChange"
	const (
		OAuthClientID     = "oauth_client_id"
		OAuthClientSecret = "oauth_client_secret"
		OAuthAddClientIDs = "oauth_additional_client_ids"
		TokenServerURL    = "token_server_url"
		SecurityConfig    = "security_config"
	)

	if getBoolProp(new, "auth_db_deleted") {
		return nil, errors.New("AuthGlobalConfig cannot be deleted")
	}

	prevClientID := ""
	prevClientSecret := ""
	prevClientIDs := []string{}
	prevTokenServerURL := ""
	prevSecurityConfig := []byte(nil)
	if old != nil {
		prevClientID = getStringProp(old, OAuthClientID)
		prevClientSecret = getStringProp(old, OAuthClientSecret)
		prevClientIDs = getStringSliceProp(old, OAuthAddClientIDs)
		prevTokenServerURL = getStringProp(old, TokenServerURL)
		prevSecurityConfig = getByteSliceProp(old, SecurityConfig)
	}

	newClientID := getStringProp(new, OAuthClientID)
	newClientSecret := getStringProp(new, OAuthClientSecret)
	newClientIDs := getStringSliceProp(new, OAuthAddClientIDs)
	newTokenServerURL := getStringProp(new, TokenServerURL)
	newSecurityConfig := getByteSliceProp(new, SecurityConfig)

	if prevClientID != newClientID || prevClientSecret != newClientSecret {
		changes = append(changes, setTargetTypeFields(ctx, ChangeConfOauthClientChanged, target, class, &AuthDBChange{OauthClientID: newClientID, OauthClientSecret: newClientSecret}))
	}

	added, removed := diffLists(prevClientIDs, newClientIDs)
	if len(added) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeConfClientIDsAdded, target, class, &AuthDBChange{OauthAdditionalClientIDs: added}))
	}
	if len(removed) > 0 {
		changes = append(changes, setTargetTypeFields(ctx, ChangeConfClientIDsRemoved, target, class, &AuthDBChange{OauthAdditionalClientIDs: removed}))
	}

	if prevTokenServerURL != newTokenServerURL {
		changes = append(changes, setTargetTypeFields(ctx, ChangeConfTokenServerURLChanged, target, class, &AuthDBChange{TokenServerURLNew: newTokenServerURL, TokenServerURLOld: prevTokenServerURL}))
	}

	if !bytes.Equal(prevSecurityConfig, newSecurityConfig) {
		changes = append(changes, setTargetTypeFields(ctx, ChangeConfSecurityConfigChanged, target, class, &AuthDBChange{SecurityConfigNew: newSecurityConfig, SecurityConfigOld: prevSecurityConfig}))
	}

	return changes, nil
}

func diffRealmsGlobals(ctx context.Context, target string, old, new datastore.PropertyMap) ([]*AuthDBChange, error) {
	changes := []*AuthDBChange{}
	class := "AuthDBRealmsGlobalsChange"

	oldRealms, err := toAuthRealmsGlobals(old)
	if err != nil {
		return changes, err
	}
	newRealms, err := toAuthRealmsGlobals(new)
	if err != nil {
		return changes, err
	}

	// Get changes for permissions maintained by Auth Service v1.
	oldNames, oldPermissions, err := extractV1Permissions(oldRealms)
	if err != nil {
		return []*AuthDBChange{}, err
	}
	newNames, newPermissions, err := extractV1Permissions(newRealms)
	if err != nil {
		return []*AuthDBChange{}, err
	}
	authChange, isChanged := diffPermissions(ctx, oldNames, newNames, oldPermissions, newPermissions)
	if isChanged {
		setTargetTypeFields(ctx, ChangeRealmsGlobalsChanged, target, class, authChange)
		changes = append(changes, authChange)
	}

	// Get changes for permissions maintained by Auth Service v2.
	oldNames, oldPermissions = extractV2Permissions(oldRealms)
	newNames, newPermissions = extractV2Permissions(newRealms)
	authChange, isChanged = diffPermissions(ctx, oldNames, newNames, oldPermissions, newPermissions)
	if isChanged {
		setTargetTypeFields(ctx, ChangeRealmsGlobalsChanged, target, class, authChange)
		changes = append(changes, authChange)
	}

	return changes, nil
}

// extractV1Permissions returns the permissions maintained by
// Auth Service v1 from the given AuthRealmsGlobals.
//
// Returns:
// - permission names in their stored order; and
// - a map of permissions, where the key is the permission name.
func extractV1Permissions(a *AuthRealmsGlobals) ([]string, map[string]*protocol.Permission, error) {
	if a == nil {
		return []string{}, map[string]*protocol.Permission{}, nil
	}

	names := make([]string, len(a.Permissions))
	permissionsByName := make(map[string]*protocol.Permission, len(a.Permissions))
	for i, p := range a.Permissions {
		permission := &protocol.Permission{}
		err := proto.Unmarshal([]byte(p), permission)
		if err != nil {
			err = errors.Annotate(err, "error while unmarshalling stored Permissions").Err()
			return []string{}, map[string]*protocol.Permission{}, err
		}
		name := permission.GetName()
		names[i] = name
		permissionsByName[name] = permission
	}
	return names, permissionsByName, nil
}

// extractV2Permissions returns the permissions maintained by
// Auth Service v2 from the given AuthRealmsGlobals.
//
// Returns:
// - permission names in their stored order; and
// - a map of permissions, where the key is the permission name.
func extractV2Permissions(a *AuthRealmsGlobals) ([]string, map[string]*protocol.Permission) {
	if a == nil || a.PermissionsList == nil {
		return []string{}, map[string]*protocol.Permission{}
	}

	permissions := a.PermissionsList.GetPermissions()
	names := make([]string, len(permissions))
	permissionsByName := make(map[string]*protocol.Permission, len(permissions))
	for i, permission := range permissions {
		name := permission.GetName()
		names[i] = name
		permissionsByName[name] = permission
	}
	return names, permissionsByName
}

// diffPermissions is a helper function to identify the differences
// between the two sets of permissions.
//
// Returns an AuthDBChange if there was actually a difference, and
// whether the permissions were different.
func diffPermissions(ctx context.Context, oldNames, newNames []string, oldPerms, newPerms map[string]*protocol.Permission) (*AuthDBChange, bool) {
	var added, changed []string
	for _, name := range newNames {
		newPerm := newPerms[name]
		oldPerm, ok := oldPerms[name]
		if !ok {
			// Permission is not in previous version.
			added = append(added, name)
		} else if oldPerm != newPerm {
			// Permission is different between versions.
			changed = append(changed, name)
		}
	}

	var removed []string
	for _, name := range oldNames {
		if _, ok := newPerms[name]; !ok {
			// Permission is not in the newer version.
			removed = append(removed, name)
		}
	}

	// No changes at all; permissions are identical.
	if len(added) == 0 && len(changed) == 0 && len(removed) == 0 {
		return nil, false
	}

	return &AuthDBChange{
		PermissionsAdded:   added,
		PermissionsChanged: changed,
		PermissionsRemoved: removed,
	}, true
}

// /////////////////////////////////////////////////////////////////////
// ///////////////// PropertyMap helper functions //////////////////////
// /////////////////////////////////////////////////////////////////////
func getProp(pm datastore.PropertyMap, key string) any {
	pd := pm[key]
	if pd == nil {
		return nil
	}
	switch v := pd.(type) {
	case datastore.Property:
		return v.Value()
	default:
		panic("getProp only supports single property values. Try getStringSliceProp() instead")
	}
}

func getDescription(pm datastore.PropertyMap) string {
	return getStringProp(pm, "description")
}

func getDatastoreKey(pm datastore.PropertyMap) *datastore.Key {
	return getProp(pm, "$key").(*datastore.Key)
}

func getBoolProp(pm datastore.PropertyMap, key string) bool {
	return getProp(pm, key).(bool)
}

func getStringSliceProp(pm datastore.PropertyMap, key string) []string {
	vals := []string{}
	ps := pm.Slice(key)
	if ps == nil {
		return nil
	}
	for _, p := range ps {
		value, err := p.Project(datastore.PTString)
		if err == nil {
			vals = append(vals, value.(string))
		}
	}
	return vals
}

func getStringProp(pm datastore.PropertyMap, key string) string {
	switch v := getProp(pm, key).(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

func getInt64Prop(pm datastore.PropertyMap, key string) int64 {
	return getProp(pm, key).(int64)
}

func getTimeProp(pm datastore.PropertyMap, key string) time.Time {
	return getProp(pm, key).(time.Time)
}

func getByteSliceProp(pm datastore.PropertyMap, key string) []byte {
	value := getProp(pm, key)
	if value == nil {
		return nil
	}
	return value.([]byte)
}

func toAuthRealmsGlobals(pm datastore.PropertyMap) (*AuthRealmsGlobals, error) {
	// Copy the map, excluding meta fields.
	filteredPM, err := pm.Save(false)
	if err != nil {
		return nil, errors.New("failed to copy PropertyMap without meta")
	}

	// Delete *History fields, if present.
	historyFields := []string{"auth_db_deleted", "auth_db_change_comment", "auth_db_app_version"}
	for _, name := range historyFields {
		delete(filteredPM, name)
	}

	a := &AuthRealmsGlobals{}
	if err := datastore.GetPLS(a).Load(filteredPM); err != nil {
		return nil, err
	}

	return a, nil
}

///////////////////////////////////////////////////////////////////////
