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
	"sort"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/tq"
)

func testAuthDBGroupChange(ctx context.Context, target string, changeType ChangeType, authDBRev int64) *AuthDBChange {
	change := &AuthDBChange{
		Kind:       "AuthDBChange",
		Parent:     ChangeLogRevisionKey(ctx, authDBRev),
		Class:      []string{"AuthDBChange", "AuthDBGroupChange"},
		ChangeType: changeType,
		Target:     target,
		AuthDBRev:  authDBRev,
		Who:        "user:test@example.com",
		When:       time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC),
		Comment:    "comment",
		AppVersion: "123-45abc",
	}

	var err error
	change.ID, err = ChangeID(ctx, change)
	So(err, ShouldBeNil)
	return change
}

func testAuthDBIPAllowlistChange(ctx context.Context, authDBRev int64) *AuthDBChange {
	change := &AuthDBChange{
		Kind:           "AuthDBChange",
		Parent:         ChangeLogRevisionKey(ctx, authDBRev),
		Class:          []string{"AuthDBChange", "AuthDBIPWhitelistChange"},
		ChangeType:     3000,
		Comment:        "comment",
		Description:    "description",
		OldDescription: "",
		Target:         "AuthIPWhitelist$a",
		When:           time.Date(2021, time.December, 12, 1, 0, 0, 0, time.UTC),
		Who:            "user:test@example.com",
		AppVersion:     "123-45abc",
	}

	var err error
	change.ID, err = ChangeID(ctx, change)
	So(err, ShouldBeNil)
	return change
}

func testAuthDBIPAllowlistAssignmentChange(ctx context.Context, authDBRev int64) *AuthDBChange {
	change := &AuthDBChange{
		Kind:        "AuthDBChange",
		Parent:      ChangeLogRevisionKey(ctx, authDBRev),
		Class:       []string{"AuthDBChange", "AuthDBIPWhitelistAssignmentChange"},
		ChangeType:  5100,
		Comment:     "comment",
		Identity:    "test",
		IPAllowlist: "test",
		Target:      "AuthIPWhitelistAssignments$default$user:test@example.com",
		When:        time.Date(2019, time.December, 12, 1, 0, 0, 0, time.UTC),
		Who:         "user:test@example.com",
		AppVersion:  "123-45abc",
	}

	var err error
	change.ID, err = ChangeID(ctx, change)
	So(err, ShouldBeNil)
	return change
}

func testAuthDBConfigChange(ctx context.Context, authDBRev int64) *AuthDBChange {
	change := &AuthDBChange{
		Kind:              "AuthDBChange",
		Parent:            ChangeLogRevisionKey(ctx, authDBRev),
		Class:             []string{"AuthDBChange", "AuthDBConfigChange"},
		ChangeType:        7000,
		Comment:           "comment",
		OauthClientID:     "123.test.example.com",
		OauthClientSecret: "aBcD",
		Target:            "AuthGlobalConfig$test",
		When:              time.Date(2019, time.December, 11, 1, 0, 0, 0, time.UTC),
		Who:               "user:test@example.com",
		AppVersion:        "123-45abc",
	}

	var err error
	change.ID, err = ChangeID(ctx, change)
	So(err, ShouldBeNil)
	return change
}

func testAuthRealmsGlobalsChange(ctx context.Context, authDBRev int64) *AuthDBChange {
	change := &AuthDBChange{
		Kind:             "AuthDBChange",
		Parent:           ChangeLogRevisionKey(ctx, authDBRev),
		Class:            []string{"AuthDBChange", "AuthRealmsGlobalsChange"},
		ChangeType:       9000,
		Comment:          "comment",
		PermissionsAdded: []string{"a.existInRealm"},
		Target:           "AuthRealmsGlobals$globals",
		When:             time.Date(2021, time.January, 11, 1, 0, 0, 0, time.UTC),
		Who:              "user:test@example.com",
		AppVersion:       "123-45abc",
	}

	var err error
	change.ID, err = ChangeID(ctx, change)
	So(err, ShouldBeNil)
	return change
}

func testAuthProjectRealmsChange(ctx context.Context, authDBRev int64) *AuthDBChange {
	change := &AuthDBChange{
		Kind:         "AuthDBChange",
		Parent:       ChangeLogRevisionKey(ctx, authDBRev),
		Class:        []string{"AuthDBChange", "AuthProjectRealmsChange"},
		ChangeType:   10200,
		Comment:      "comment",
		ConfigRevOld: "",
		ConfigRevNew: "",
		PermsRevOld:  "auth_service_ver:100-00abc",
		PermsRevNew:  "auth_service_ver:123-45abc",
		Target:       "AuthProejctRealms$repo",
		When:         time.Date(2019, time.January, 11, 1, 0, 0, 0, time.UTC),
		Who:          "user:test@example.com",
		AppVersion:   "123-45abc",
	}

	var err error
	change.ID, err = ChangeID(ctx, change)
	So(err, ShouldBeNil)
	return change
}

////////////////////////////////////////////////////////////////////////////////

func TestGetAllAuthDBChange(t *testing.T) {
	t.Parallel()
	Convey("Testing GetAllAuthDBChange", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		So(datastore.Put(ctx,
			testAuthDBGroupChange(ctx, "AuthGroup$groupA", ChangeGroupCreated, 1120),
			testAuthDBGroupChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1120),
			testAuthDBGroupChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
			testAuthDBGroupChange(ctx, "AuthGroup$groupC", ChangeGroupMembersAdded, 1135),
			testAuthDBGroupChange(ctx, "AuthGroup$groupC", ChangeGroupOwnersChanged, 1110),
			testAuthDBIPAllowlistChange(ctx, 1120),
			testAuthDBIPAllowlistAssignmentChange(ctx, 1121),
			testAuthDBConfigChange(ctx, 1115),
			testAuthRealmsGlobalsChange(ctx, 1120),
			testAuthProjectRealmsChange(ctx, 1115),
		), ShouldBeNil)

		Convey("Sort by key", func() {
			changes, pageToken, err := GetAllAuthDBChange(ctx, "", 0, 5, "")
			So(err, ShouldBeNil)
			So(pageToken, ShouldNotBeEmpty)
			So(changes, ShouldResemble, []*AuthDBChange{
				testAuthDBGroupChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
				testAuthDBGroupChange(ctx, "AuthGroup$groupC", ChangeGroupMembersAdded, 1135),
				testAuthDBIPAllowlistAssignmentChange(ctx, 1121),
				testAuthRealmsGlobalsChange(ctx, 1120),
				testAuthDBIPAllowlistChange(ctx, 1120),
			})

			changes, _, err = GetAllAuthDBChange(ctx, "", 0, 5, pageToken)
			So(err, ShouldBeNil)
			So(changes, ShouldResemble, []*AuthDBChange{
				testAuthDBGroupChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1120),
				testAuthDBGroupChange(ctx, "AuthGroup$groupA", ChangeGroupCreated, 1120),
				testAuthProjectRealmsChange(ctx, 1115),
				testAuthDBConfigChange(ctx, 1115),
				testAuthDBGroupChange(ctx, "AuthGroup$groupC", ChangeGroupOwnersChanged, 1110),
			})
		})
		Convey("Filter by target", func() {
			changes, _, err := GetAllAuthDBChange(ctx, "AuthGroup$groupB", 0, 10, "")
			So(err, ShouldBeNil)
			So(changes, ShouldResemble, []*AuthDBChange{
				testAuthDBGroupChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
				testAuthDBGroupChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1120),
			})
		})
		Convey("Filter by authDBRev", func() {
			changes, _, err := GetAllAuthDBChange(ctx, "", 1136, 10, "")
			So(err, ShouldBeNil)
			So(changes, ShouldResemble, []*AuthDBChange{
				testAuthDBGroupChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
			})
		})
		Convey("Return error when target is invalid", func() {
			_, _, err := GetAllAuthDBChange(ctx, "groupname", 0, 10, "")
			So(err, ShouldErrLike, "Invalid target groupname")
		})
	})
}

func TestGenerateChanges(t *testing.T) {
	t.Parallel()

	Convey("GenerateChanges", t, func() {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{AdminGroup},
		})
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		datastore.Put(ctx, testAuthGroup(ctx, AdminGroup))

		//////////////////////////////////////////////////////////
		// Helper functions
		getChanges := func(ctx context.Context, authDBRev int64) []*AuthDBChange {
			var ancestor *datastore.Key
			if authDBRev != 0 {
				ancestor = ChangeLogRevisionKey(ctx, authDBRev)
			} else {
				ancestor = ChangeLogRootKey(ctx)
			}
			query := datastore.NewQuery("AuthDBChange").Ancestor(ancestor)
			changes := []*AuthDBChange{}
			datastore.Run(ctx, query, func(change *AuthDBChange) {
				changes = append(changes, change)
			})
			return changes
		}

		validateChanges := func(ctx context.Context, msg string, actualChanges []*AuthDBChange, authDBRev int64, cts ...ChangeType) {
			SoMsg(msg, actualChanges, ShouldHaveLength, len(cts))
			for i, ct := range actualChanges {
				SoMsg(msg, ct.ChangeType, ShouldEqual, cts[i])
			}
			sort.Slice(actualChanges, func(i, j int) bool {
				return actualChanges[i].ChangeType < actualChanges[j].ChangeType
			})
			SoMsg(msg, actualChanges, ShouldResemble, getChanges(ctx, authDBRev))
		}
		//////////////////////////////////////////////////////////

		Convey("AuthGroup changes", func() {
			Convey("AuthGroup Created/Deleted", func() {
				ag1 := makeAuthGroup(ctx, "group-1")
				_, err := CreateAuthGroup(ctx, ag1, false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create group", actualChanges, 1, ChangeGroupCreated)

				So(DeleteAuthGroup(ctx, ag1.ID, "", false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete group", actualChanges, 2, ChangeGroupDeleted)
			})

			Convey("AuthGroup Owners / Description changed", func() {
				og := makeAuthGroup(ctx, "owning-group")
				So(datastore.Put(ctx, og), ShouldBeNil)

				ag1 := makeAuthGroup(ctx, "group-1")
				_, err := CreateAuthGroup(ctx, ag1, false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create group no owners", actualChanges, 1, ChangeGroupCreated)

				ag1.Owners = og.ID
				ag1.Description = "test-desc"
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group owners & desc", actualChanges, 2, ChangeGroupDescriptionChanged, ChangeGroupOwnersChanged)

				ag1.Description = "new-desc"
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group desc", actualChanges, 3, ChangeGroupDescriptionChanged)
			})

			Convey("AuthGroup add/remove Members", func() {
				// Add members in Create
				ag1 := makeAuthGroup(ctx, "group-1")
				ag1.Members = []string{"user:someone@example.com"}
				_, err := CreateAuthGroup(ctx, ag1, false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create group +mems", actualChanges, 1, ChangeGroupCreated, ChangeGroupMembersAdded)

				// Add members to already existing group
				ag1.Members = append(ag1.Members, "user:another-one@example.com")
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group +mems", actualChanges, 2, ChangeGroupMembersAdded)

				// Remove members from existing group
				ag1.Members = []string{"user:someone@example.com"}
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group -mems", actualChanges, 3, ChangeGroupMembersRemoved)

				// Remove members when deleting group
				So(DeleteAuthGroup(ctx, ag1.ID, "", false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 8)
				actualChanges, err = generateChanges(ctx, 4, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete group -mems", actualChanges, 4, ChangeGroupMembersRemoved, ChangeGroupDeleted)
			})

			Convey("AuthGroup add/remove globs", func() {
				// Add globs in create
				ag1 := makeAuthGroup(ctx, "group-1")
				ag1.Globs = []string{"user:*@example.com"}
				_, err := CreateAuthGroup(ctx, ag1, false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create group +globs", actualChanges, 1, ChangeGroupCreated, ChangeGroupGlobsAdded)

				// Add globs to already existing group
				ag1.Globs = append(ag1.Globs, "user:test-*@test.com")
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group +globs", actualChanges, 2, ChangeGroupGlobsAdded)

				ag1.Globs = []string{"user:test-*@test.com"}
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				So(err, ShouldBeNil)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group -globs", actualChanges, 3, ChangeGroupGlobsRemoved)

				So(DeleteAuthGroup(ctx, ag1.ID, "", false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 8)
				actualChanges, err = generateChanges(ctx, 4, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete group -globs", actualChanges, 4, ChangeGroupGlobsRemoved, ChangeGroupDeleted)
			})

			Convey("AuthGroup add/remove nested", func() {
				ag2 := makeAuthGroup(ctx, "group-2")
				ag3 := makeAuthGroup(ctx, "group-3")
				So(datastore.Put(ctx, ag2, ag3), ShouldBeNil)

				// Add globs in create
				ag1 := makeAuthGroup(ctx, "group-1")
				ag1.Nested = []string{ag2.ID}
				_, err := CreateAuthGroup(ctx, ag1, false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create group +nested", actualChanges, 1, ChangeGroupCreated, ChangeGroupNestedAdded)

				// Add globs to already existing group
				ag1.Nested = append(ag1.Nested, ag3.ID)
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group +nested", actualChanges, 2, ChangeGroupNestedAdded)

				ag1.Nested = []string{"group-2"}
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				So(err, ShouldBeNil)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group -nested", actualChanges, 3, ChangeGroupNestedRemoved)

				So(DeleteAuthGroup(ctx, ag1.ID, "", false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 8)
				actualChanges, err = generateChanges(ctx, 4, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete group -nested", actualChanges, 4, ChangeGroupNestedRemoved, ChangeGroupDeleted)
			})
		})

		Convey("AuthIPAllowlist changes", func() {
			Convey("AuthIPAllowlist Created/Deleted +/- subnets", func() {
				// Creation with no subnet
				baseSubnetMap := make(map[string][]string)
				baseSubnetMap["test-allowlist-1"] = []string{}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create allowlist", actualChanges, 1, ChangeIPALCreated)

				// Deletion with no subnet
				baseSubnetMap = map[string][]string{}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete allowlist", actualChanges, 2, ChangeIPALDeleted)

				// Creation with subnets
				baseSubnetMap["test-allowlist-1"] = []string{"123.4.5.6"}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create allowlist w/ subnet", actualChanges, 3, ChangeIPALCreated, ChangeIPALSubnetsAdded)

				// Add subnet
				baseSubnetMap["test-allowlist-1"] = append(baseSubnetMap["test-allowlist-1"], "567.8.9.10")
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 8)
				actualChanges, err = generateChanges(ctx, 4, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "add subnet", actualChanges, 4, ChangeIPALSubnetsAdded)

				// Remove subnet
				baseSubnetMap["test-allowlist-1"] = baseSubnetMap["test-allowlist-1"][1:]
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 10)
				actualChanges, err = generateChanges(ctx, 5, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "remove subnet", actualChanges, 5, ChangeIPALSubnetsRemoved)

				// Delete allowlist with subnet
				baseSubnetMap = map[string][]string{}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 12)
				actualChanges, err = generateChanges(ctx, 6, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete allowlist w/ subnet", actualChanges, 6, ChangeIPALDeleted, ChangeIPALSubnetsRemoved)
			})

			Convey("AuthIPAllowlist description changed", func() {
				baseSubnetMap := make(map[string][]string)
				baseSubnetMap["test-allowlist-1"] = []string{}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				_, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)

				al, err := GetAuthIPAllowlist(ctx, "test-allowlist-1")
				So(err, ShouldBeNil)
				So(runAuthDBChange(ctx, func(ctx context.Context, cae commitAuthEntity) error {
					al.Description = "new-desc"
					return cae(al, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), false)
				}), ShouldBeNil)
				actualChanges, err := generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "change description", actualChanges, 2, ChangeIPALDescriptionChanged)
			})
		})

		Convey("AuthGlobalConfig changes", func() {
			Convey("AuthGlobalConfig ClientID/ClientSecret mismatch", func() {
				baseCfg := &configspb.OAuthConfig{
					PrimaryClientId:     "test-client-id",
					PrimaryClientSecret: "test-client-secret",
				}

				// Old doesn't exist yet
				So(UpdateAuthGlobalConfig(ctx, baseCfg, nil, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with no old config present", actualChanges, 1, ChangeConfOauthClientChanged)

				newCfg := &configspb.OAuthConfig{
					PrimaryClientId: "diff-client-id",
				}
				So(UpdateAuthGlobalConfig(ctx, newCfg, nil, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with client id changed", actualChanges, 2, ChangeConfOauthClientChanged)
			})

			Convey("AuthGlobalConfig additional +/- ClientID's", func() {
				baseCfg := &configspb.OAuthConfig{
					ClientIds: []string{"test.example.com"},
				}

				So(UpdateAuthGlobalConfig(ctx, baseCfg, nil, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with client ids, old config not present", actualChanges, 1, ChangeConfClientIDsAdded)

				newCfg := &configspb.OAuthConfig{
					ClientIds: []string{"not-test.example.com"},
				}
				So(UpdateAuthGlobalConfig(ctx, newCfg, nil, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with client id added and client id removed, old config is present",
					actualChanges, 2, ChangeConfClientIDsAdded, ChangeConfClientIDsRemoved)
			})

			Convey("AuthGlobalConfig TokenServerURL change", func() {
				baseCfg := &configspb.OAuthConfig{
					TokenServerUrl: "test-token-server-url.example.com",
				}

				So(UpdateAuthGlobalConfig(ctx, baseCfg, nil, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with token server url, old config not present", actualChanges, 1, ChangeConfTokenServerURLChanged)
			})

			Convey("AuthGlobalConfig Security Config change", func() {
				baseCfg := &configspb.OAuthConfig{}
				secCfg := &protocol.SecurityConfig{
					InternalServiceRegexp: []string{"abc"},
				}
				So(UpdateAuthGlobalConfig(ctx, baseCfg, secCfg, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with security config, old config not present", actualChanges, 1, ChangeConfSecurityConfigChanged)

				secCfg = &protocol.SecurityConfig{
					InternalServiceRegexp: []string{"def"},
				}
				So(UpdateAuthGlobalConfig(ctx, baseCfg, secCfg, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with security config, old config present", actualChanges, 2, ChangeConfSecurityConfigChanged)

			})

			Convey("AuthGlobalConfig all changes at once", func() {
				baseCfg := &configspb.OAuthConfig{
					PrimaryClientId:     "test-client-id",
					PrimaryClientSecret: "test-client-secret",
					ClientIds:           []string{"a", "b", "c"},
					TokenServerUrl:      "token-server.example.com",
				}
				secCfg := &protocol.SecurityConfig{
					InternalServiceRegexp: []string{"test"},
				}
				So(UpdateAuthGlobalConfig(ctx, baseCfg, secCfg, false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "all changes at once, old config not present", actualChanges, 1, ChangeConfOauthClientChanged, ChangeConfClientIDsAdded, ChangeConfTokenServerURLChanged, ChangeConfSecurityConfigChanged)
			})
		})
	})
}
