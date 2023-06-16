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
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/api/taskspb"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"
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
	OauthClientID            string   `gae:"oauth_client_id"`             // Valid for ChangeConfOauthClientChanged.
	OauthClientSecret        string   `gae:"oauth_client_secret"`         // Valid for ChangeConfOauthClientChanged.
	OauthAdditionalClientIDs []string `gae:"oauth_additional_client_ids"` // Valid for ChangeConfClientIDsAdded and ChangeConfClientIDsRemoved.
	TokenServerURLOld        string   `gae:"token_server_url_old"`        // Valid for ChangeConfTokenServerURLChanged.
	TokenServerURLNew        string   `gae:"token_server_url_new"`        // Valid for ChangeConfTokenServerURLChanged.
	SecurityConfigOld        []byte   `gae:"security_config_old,noindex"` // Valid for ChangeConfSecurityConfigChanged.
	SecurityConfigNew        []byte   `gae:"security_config_new,noindex"` // Valid for ChangeConfSecurityConfigChanged.

	// Fields specific to AuthRealmsGlobalsChange.
	PermissionsAdded   []string `gae:"permissions_added"`
	PermissionsChanged []string `gae:"permissions_changed"`
	PermissionsRemoved []string `gae:"permissions_removed"`

	// Fields specific to AuthProjectRealmsChange.
	ConfigRevOld string `gae:"config_rev_old,noindex"`
	ConfigRevNew string `gae:"config_rev_new,noindex"`
	PermsRevOld  string `gae:"perms_rev_old,noindex"`
	PermsRevNew  string `gae:"perms_rev_new,noindex"`
}

// ChangeLogRootKey returns the root key of an entity group with change log.
func ChangeLogRootKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "AuthDBLog", "v1", 0, nil)
}

// ChangeLogRevisionKey returns the key of entity subgroup that keeps AuthDB
// change log for a revision.
func ChangeLogRevisionKey(ctx context.Context, authDBRev int64) *datastore.Key {
	return datastore.NewKey(ctx, "AuthDBLogRev", "", authDBRev, ChangeLogRootKey(ctx))
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
	var ancestor *datastore.Key
	if authDBRev != 0 {
		ancestor = ChangeLogRevisionKey(ctx, authDBRev)
	} else {
		ancestor = ChangeLogRootKey(ctx)
	}

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

func handleProcessChangeTask(ctx context.Context, task *taskspb.ProcessChangeTask) error {
	// TODO(cjacomet): Implement
	return nil
}

func ensureInitialSnapshot() {
	// TODO(cjacomet): Implement
}

func generateChanges(ctx context.Context, authDBRev int64, dryRun bool) ([]*AuthDBChange, error) {
	query := datastore.NewQuery("").Ancestor(HistoricalRevisionKey(ctx, authDBRev))
	changes := []*AuthDBChange{}

	getPms := func(key *datastore.Key, class string, ch *AuthDBChange, pm datastore.PropertyMap) (string, datastore.PropertyMap, datastore.PropertyMap, error) {
		target := fmt.Sprintf("%s$%s", class, key.StringID())
		ch.ID = target
		if err := datastore.Get(ctx, ch); err == nil {
			return "", nil, nil, err
		}
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

	err := datastore.Run(ctx, query, func(pm datastore.PropertyMap) error {
		c := &AuthDBChange{
			Kind:   "AuthDBChange",
			Parent: ChangeLogRevisionKey(ctx, authDBRev),
		}

		k, ok := pm.GetMeta("key")
		if !ok {
			return errors.New("key meta doesn't exist")
		}
		switch key := k.(*datastore.Key); {
		case strings.Contains(key.String(), "AuthGroupHistory"):
			target, oldpm, pm, err := getPms(key, "AuthGroup", c, pm)
			if err != nil {
				return err
			}

			groupChanges, err := diffGroups(ctx, target, oldpm, pm)
			if err != nil {
				return err
			}
			changes = append(changes, groupChanges...)
		case strings.Contains(key.String(), "AuthIPWhitelistHistory"):
			target, oldpm, pm, err := getPms(key, "AuthIPWhitelist", c, pm)
			if err != nil {
				return err
			}

			ipaChanges, err := diffIPAllowlists(ctx, target, oldpm, pm)
			if err != nil {
				return err
			}
			changes = append(changes, ipaChanges...)
		case strings.Contains(key.String(), "AuthIPWhitelistAssignmentsHistory"):
			// TODO(cjacomet): Implement!
		case strings.Contains(key.String(), "AuthGlobalConfigHistory"):
			// TODO(cjacomet): Implement!
		case strings.Contains(key.String(), "AuthRealmsGlobalHistory"):
			// TODO(cjacomet): Implement!
		case strings.Contains(key.String(), "AuthProjectRealmsHistory"):
			// TODO(cjacomet): Implement!
		default:
			return fmt.Errorf("history entity not supported %s", key.String())
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	if dryRun {
		logging.Infof(ctx, "dryRun: changes generated %v", changes)
	} else {
		if err := datastore.Put(ctx, changes); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

func diffLists(old, new []string) ([]string, []string) {
	oldss := stringset.NewFromSlice(old...)
	newss := stringset.NewFromSlice(new...)
	return newss.Difference(oldss).ToSortedSlice(), oldss.Difference(newss).ToSortedSlice()
}

func makeChange(ctx context.Context, ct ChangeType, target string, authDBRev int64, class string, kwargs ...string) *AuthDBChange {
	var ancestor *datastore.Key
	if authDBRev != 0 {
		ancestor = ChangeLogRevisionKey(ctx, authDBRev)
	} else {
		ancestor = ChangeLogRootKey(ctx)
	}
	a := &AuthDBChange{
		Kind:       "AuthDBChange",
		ID:         fmt.Sprintf("%s!%d", target, ct),
		Parent:     ancestor,
		Class:      []string{"AuthDBChange", class},
		ChangeType: ct,
		Target:     target,
		AuthDBRev:  authDBRev,
		Who:        string(auth.CurrentIdentity(ctx)),
		When:       clock.Now(ctx).UTC(),
	}

	jsonMap := make(map[string]interface{})

	for _, kwarg := range kwargs {
		k, v, ok := strings.Cut(kwarg, "=")
		if !ok {
			logging.Warningf(ctx, "kwarg has incorrect format: %s, should be k=v, where k is a string, v can be any value", kwarg)
		}
		jsonMap[k] = v
	}
	b, err := json.Marshal(jsonMap)
	if err != nil {
		logging.Errorf(ctx, "failed trying to marshal json map for AuthChange: %s", err.Error())
	}
	if err = json.Unmarshal(b, a); err != nil {
		logging.Errorf(ctx, "failed trying to unmarshal json for AuthChange: %s", err.Error())
	}

	return a
}

func kvPair(k string, v any) string {
	return fmt.Sprintf("%s=%v", k, v)
}

func diffGroups(ctx context.Context, target string, old, new datastore.PropertyMap) ([]*AuthDBChange, error) {
	changes := []*AuthDBChange{}
	authDBRev := getInt64Prop(new, "auth_db_rev")
	class := "AuthDBGroupChange"

	if getBoolProp(new, "auth_db_deleted") {
		if mems := getStringSliceProp(new, "members"); mems != nil {
			changes = append(changes, makeChange(ctx, ChangeGroupMembersRemoved, target, authDBRev, class, kvPair("members", mems)))
		}
		if globs := getStringSliceProp(new, "globs"); globs != nil {
			changes = append(changes, makeChange(ctx, ChangeGroupGlobsRemoved, target, authDBRev, class, kvPair("globs", globs)))
		}
		if nested := getStringSliceProp(new, "nested"); nested != nil {
			changes = append(changes, makeChange(ctx, ChangeGroupNestedRemoved, target, authDBRev, class, kvPair("nested", nested)))
		}
		desc := ""
		if getProp(new, "description") != nil {
			desc = getStringProp(new, "description")
		}
		owners := AdminGroup
		if getProp(new, "owners") != nil {
			owners = getStringProp(new, "owners")
		}
		changes = append(changes, makeChange(ctx, ChangeGroupDeleted, target, authDBRev, class, kvPair("old_description", desc), kvPair("owners", owners)))
		return changes, nil
	}

	if old == nil {
		desc := ""
		if getProp(new, "description") != nil {
			desc = getStringProp(new, "description")
		}
		owners := AdminGroup
		if getProp(new, "owners") != nil {
			owners = getStringProp(new, "owners")
		}

		changes = append(changes, makeChange(ctx, ChangeGroupCreated, target, authDBRev, class, kvPair("description", desc), kvPair("owners", owners)))
		if mems := getStringSliceProp(new, "members"); mems != nil {
			changes = append(changes, makeChange(ctx, ChangeGroupMembersAdded, target, authDBRev, class, kvPair("members", mems)))
		}
		if globs := getStringSliceProp(new, "globs"); globs != nil {
			changes = append(changes, makeChange(ctx, ChangeGroupGlobsAdded, target, authDBRev, class, kvPair("globs", globs)))
		}
		if nested := getStringSliceProp(new, "nested"); nested != nil {
			changes = append(changes, makeChange(ctx, ChangeGroupNestedAdded, target, authDBRev, class, kvPair("nested", nested)))
		}
		return changes, nil
	}

	if oldDesc, newDesc := getProp(old, "description"), getProp(new, "description"); oldDesc != newDesc {
		d, od := "", ""
		if newDesc != nil {
			d = getStringProp(new, "description")
		}
		if oldDesc != nil {
			od = getStringProp(old, "description")
		}
		changes = append(changes, makeChange(ctx, ChangeGroupDescriptionChanged, target, authDBRev, class, kvPair("description", d), kvPair("old_description", od)))
	}

	oldOwners := AdminGroup
	if getProp(old, "owners") != nil {
		oldOwners = getStringProp(old, "owners")
	}
	newOwners := getStringProp(new, "owners")
	if oldOwners != newOwners {
		changes = append(changes, makeChange(ctx, ChangeGroupOwnersChanged, target, authDBRev, class, kvPair("old_owners", oldOwners), kvPair("owners", newOwners)))
	}

	added, removed := diffLists(getStringSliceProp(old, "members"), getStringSliceProp(new, "members"))
	if len(added) > 0 {
		changes = append(changes, makeChange(ctx, ChangeGroupMembersAdded, target, authDBRev, class, kvPair("members", added)))
	}
	if len(removed) > 0 {
		changes = append(changes, makeChange(ctx, ChangeGroupMembersRemoved, target, authDBRev, class, kvPair("members", removed)))
	}

	added, removed = diffLists(getStringSliceProp(old, "globs"), getStringSliceProp(new, "globs"))
	if len(added) > 0 {
		changes = append(changes, makeChange(ctx, ChangeGroupGlobsAdded, target, authDBRev, class, kvPair("globs", added)))
	}
	if len(removed) > 0 {
		changes = append(changes, makeChange(ctx, ChangeGroupGlobsRemoved, target, authDBRev, class, kvPair("globs", removed)))
	}

	added, removed = diffLists(getStringSliceProp(old, "nested"), getStringSliceProp(new, "nested"))
	if len(added) > 0 {
		changes = append(changes, makeChange(ctx, ChangeGroupNestedAdded, target, authDBRev, class, kvPair("nested", added)))
	}
	if len(removed) > 0 {
		changes = append(changes, makeChange(ctx, ChangeGroupNestedRemoved, target, authDBRev, class, kvPair("nested", removed)))
	}

	return changes, nil
}

func diffIPAllowlists(ctx context.Context, target string, old, new datastore.PropertyMap) ([]*AuthDBChange, error) {
	changes := []*AuthDBChange{}
	authDBRev := getInt64Prop(new, "auth_db_rev")
	class := "AuthDBIPWhitelistChange"
	if getBoolProp(new, "auth_db_deleted") {
		d := getDescription(new)
		changes = append(changes, makeChange(ctx,
			ChangeIPALDeleted, target, authDBRev, class, kvPair("old_description", d)))
		if subnets := getStringSliceProp(new, "subnets"); subnets != nil {
			changes = append(changes, makeChange(ctx,
				ChangeIPALSubnetsRemoved, target, authDBRev, class, kvPair("subnets", subnets)))
		}
		return changes, nil
	}

	if old == nil {
		d := getDescription(new)
		changes = append(changes, makeChange(ctx,
			ChangeIPALCreated, target, authDBRev, class, kvPair("description", d)))
		if subnets := getStringSliceProp(new, "subnets"); subnets != nil {
			changes = append(changes, makeChange(ctx,
				ChangeIPALSubnetsAdded, target, authDBRev, class, kvPair("subnets", subnets)))
		}
		return changes, nil
	}

	if getDescription(old) != getDescription(new) {
		changes = append(changes, makeChange(ctx,
			ChangeIPALDescriptionChanged, target, authDBRev, class,
			kvPair("description", getDescription(new)), kvPair("old_description", getDescription(old))))
	}

	added, removed := diffLists(getStringSliceProp(old, "subnets"), getStringSliceProp(new, "subnets"))
	if len(added) > 0 {
		changes = append(changes, makeChange(ctx,
			ChangeIPALSubnetsAdded, target, authDBRev, class, kvPair("subnets", added)))
	}
	if len(removed) > 0 {
		changes = append(changes, makeChange(ctx,
			ChangeIPALSubnetsRemoved, target, authDBRev, class, kvPair("subnets", removed)))
	}
	return changes, nil
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
	switch getProp(pm, "description").(type) {
	case string:
		return getStringProp(pm, "description")
	default:
		return ""
	}
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
		vals = append(vals, p.Value().(string))
	}
	return vals
}

func getStringProp(pm datastore.PropertyMap, key string) string {
	return getProp(pm, key).(string)
}

func getInt64Prop(pm datastore.PropertyMap, key string) int64 {
	return getProp(pm, key).(int64)
}

func getTimeProp(pm datastore.PropertyMap, key string) time.Time {
	return getProp(pm, key).(time.Time)
}

func getByteSliceProp(pm datastore.PropertyMap, key string) []byte {
	return getProp(pm, key).([]byte)
}

///////////////////////////////////////////////////////////////////////
