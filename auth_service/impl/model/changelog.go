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
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
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
)

// AuthDBChange is the base (embedded) struct for change log entries.
// Has a change type and a bunch of common change properties (like who and when
// made the change). Change type order is important, it is used in UI when
// sorting changes introduces by single AuthDB commit.
//
// Change types represent minimal indivisible changes. Large AuthDB change is
// usually represented by (unordered) set of indivisible changes. For example,
// an act of creation of a new AuthDB group produces following changes:
//   ChangeGroupCreated
//   ChangeGroupMembersAdded
//   ChangeGroupGlobsAdded
//   ChangeGroupNestedAdded
//
// They are unordered, but UI sorts them based on change type integer. Thus
// ChangeGroupCreated appears on top: it's represented by the smallest integer.
//
// Entity id has following format:
//   <original_entity_kind>$<original_id>[$<subentity_id>]!<change_type>
// where:
//   original_entity_kind: a kind of modified AuthDB entity (e.g 'AuthGroup')
//   original_id: ID of modified AuthDB entity (e.g. 'Group name')
//   subentity_id: optional identified of modified part of the entity, used for
//     IP allowlist assignments entity (since it's just one big singleton).
//   change_type: integer Change* (see below), e.g. '1100'.
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
	ChangeType     ChangeType `gae:"change_type"` // What kind of a change this is (see Change*)
	Target         string     `gae:"target"`      // Entity (or subentity) that was changed: kind$id[$subid] (subid is optional).
	AuthDBRev      int64      `gae:"auth_db_rev"` // AuthDB revision at which the change was made.
	Who            string     `gae:"who"`         // Who made the change.
	When           time.Time  `gae:"when"`        // When the change was made.
	Comment        string     `gae:"comment"`     // Comment passed to record_revision or record_deletion.
	AppVersion     string     `gae:"app_version"` // GAE application version at which the change was made.
	Description    string     `gae:"description,noindex"`
	OldDescription string     `gae:"old_description,noindex"`

	// Fields specific to AuthDBGroupChange.
	Owners    string   `gae:"owners"`     // Valid for ChangeGroupCreated and ChangeGroupOwnersChanged.
	OldOwners string   `gae:"old_owners"` // Valid for ChangeGroupOwnersChanged and ChangeGroupDeleted.
	Members   []string `gae:"members"`    // Valid for ChangeGroupMembersAdded and ChangeGroupMembersRemoved.
	Globs     []string `gae:"globs"`      // Valid for ChangeGroupGlobsAdded and ChangeGroupGlobsRemoved.
	Nested    []string `gae:"nested"`     // Valid for ChangeGroupNestedAdded and ChangeGroupNestedRemoved.

	// Fields specific to AuthDBIPAllowlistChange.
	Subnets []string `gae:"subnets"` // Valid for ChangeIPWLSubnetsAdded and ChangeIPWLSubnetsRemoved.

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
func GetAllAuthDBChange(ctx context.Context, target string, authDBRev int64) ([]*AuthDBChange, error) {
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
			return nil, errors.Reason("Invalid target %s", target).Err()
		}
		query = query.Eq("target", target)
	}

	var changes []*AuthDBChange
	err := datastore.GetAll(ctx, query, &changes)
	if err != nil {
		return nil, errors.Annotate(err, "error getting all AuthDBChange entities").Err()
	}
	return changes, nil
}
