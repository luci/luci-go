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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/impl/info"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func testAuthDBGroupChange(ctx context.Context, target string, changeType ChangeType, authDBRev int64) *AuthDBChange {
	change := &AuthDBChange{
		Kind:       "AuthDBChange",
		Parent:     ChangeLogRevisionKey(ctx, authDBRev, false),
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
		Parent:         ChangeLogRevisionKey(ctx, authDBRev, false),
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
		Parent:      ChangeLogRevisionKey(ctx, authDBRev, false),
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
		Parent:            ChangeLogRevisionKey(ctx, authDBRev, false),
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
		Parent:           ChangeLogRevisionKey(ctx, authDBRev, false),
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
		Parent:       ChangeLogRevisionKey(ctx, authDBRev, false),
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
		getChanges := func(ctx context.Context, authDBRev int64, dryRun bool) []*AuthDBChange {
			ancestor := constructLogRevisionKey(ctx, authDBRev, dryRun)
			query := datastore.NewQuery(entityKind("AuthDBChange", dryRun)).Ancestor(ancestor)
			changes := []*AuthDBChange{}
			datastore.Run(ctx, query, func(change *AuthDBChange) {
				changes = append(changes, change)
			})
			return changes
		}

		// The fields in AuthDBChange to ignore when comparing results;
		// these fields are not signficant to the test cases below.
		ignoredAuthDBChangeFields := cmpopts.IgnoreFields(AuthDBChange{},
			"Kind", "ID", "Parent", "Class", "Target", "Who", "When", "Comment", "AppVersion")

		validateChanges := func(ctx context.Context, msg string, authDBRev int64, actualChanges []*AuthDBChange, expectedChanges []*AuthDBChange) {
			changeCount := len(expectedChanges)
			SoMsg(msg, actualChanges, ShouldHaveLength, changeCount)

			// Check each actual and exxpected changes are similar.
			for i := 0; i < changeCount; i++ {
				// Set the expected AuthDB revision to the given value.
				expectedChanges[i].AuthDBRev = authDBRev

				diff := cmp.Diff(actualChanges[i], expectedChanges[i], ignoredAuthDBChangeFields)
				if diff != "" {
					t.Errorf("%s - difference at index %d: %s", msg, i, diff)
				}
			}

			// Check AuthDBChange records are in datastore.
			sort.Slice(actualChanges, func(i, j int) bool {
				return actualChanges[i].ChangeType < actualChanges[j].ChangeType
			})
			SoMsg(msg, getChanges(ctx, authDBRev, false), ShouldResemble, actualChanges)
		}
		//////////////////////////////////////////////////////////

		Convey("AuthGroup changes", func() {
			Convey("AuthGroup Created/Deleted", func() {
				ag1 := makeAuthGroup(ctx, "group-1")
				_, err := CreateAuthGroup(ctx, ag1, false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				// Check that common fields were set as expected; these fields
				// are ignored for most other test cases.
				So(actualChanges, ShouldResembleProto, []*AuthDBChange{{
					ID:         "AuthGroup$group-1!1000",
					Class:      []string{"AuthDBChange", "AuthDBGroupChange"},
					ChangeType: ChangeGroupCreated,
					Target:     "AuthGroup$group-1",
					Kind:       "AuthDBChange",
					Parent:     ChangeLogRevisionKey(ctx, 1, false),
					AuthDBRev:  1,
					Who:        "user:someone@example.com",
					When:       testCreatedTS,
					Comment:    "Go pRPC API",
					AppVersion: "test-version",
					Owners:     AdminGroup,
				}})
				validateChanges(ctx, "create group", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}})

				So(DeleteAuthGroup(ctx, ag1.ID, "", false, "Go pRPC API", false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete group", 2, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupDeleted,
					OldOwners:  AdminGroup,
				}})

				// Check calling generateChanges for an already-processed
				// AuthDB revision does not make duplicate changes.
				repeated, err := generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				So(repeated, ShouldBeNil)
				// Check the changelog for the revision actually exists.
				expectedStoredChanges := []*AuthDBChange(actualChanges)
				sort.Slice(expectedStoredChanges, func(i, j int) bool {
					return expectedStoredChanges[i].ChangeType < expectedStoredChanges[j].ChangeType
				})
				So(getChanges(ctx, 2, false), ShouldResemble, expectedStoredChanges)
			})

			Convey("AuthGroup Owners / Description changed", func() {
				og := makeAuthGroup(ctx, "owning-group")
				So(datastore.Put(ctx, og), ShouldBeNil)

				ag1 := makeAuthGroup(ctx, "group-1")
				_, err := CreateAuthGroup(ctx, ag1, false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create group no owners", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}})

				ag1.Owners = og.ID
				ag1.Description = "test-desc"
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group owners & desc", 2, actualChanges, []*AuthDBChange{{
					ChangeType:  ChangeGroupDescriptionChanged,
					Description: "test-desc",
				}, {
					ChangeType: ChangeGroupOwnersChanged,
					Owners:     "owning-group",
					OldOwners:  AdminGroup,
				}})

				ag1.Description = "new-desc"
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group desc", 3, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeGroupDescriptionChanged,
					Description:    "new-desc",
					OldDescription: "test-desc",
				}})
			})

			Convey("AuthGroup add/remove Members", func() {
				// Add members in Create
				ag1 := makeAuthGroup(ctx, "group-1")
				ag1.Members = []string{"user:someone@example.com"}
				_, err := CreateAuthGroup(ctx, ag1, false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create group +mems", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}, {
					ChangeType: ChangeGroupMembersAdded,
					Members:    []string{"user:someone@example.com"},
				}})

				// Add members to already existing group
				ag1.Members = append(ag1.Members, "user:another-one@example.com")
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group +mems", 2, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupMembersAdded,
					Members:    []string{"user:another-one@example.com"},
				}})

				// Remove members from existing group
				ag1.Members = []string{"user:someone@example.com"}
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group -mems", 3, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupMembersRemoved,
					Members:    []string{"user:another-one@example.com"},
				}})

				// Remove members when deleting group
				So(DeleteAuthGroup(ctx, ag1.ID, "", false, "Go pRPC API", false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 8)
				actualChanges, err = generateChanges(ctx, 4, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete group -mems", 4, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupMembersRemoved,
					Members:    []string{"user:someone@example.com"},
				}, {
					ChangeType: ChangeGroupDeleted,
					OldOwners:  AdminGroup,
				}})
			})

			Convey("AuthGroup add/remove globs", func() {
				// Add globs in create
				ag1 := makeAuthGroup(ctx, "group-1")
				ag1.Globs = []string{"user:*@example.com"}
				_, err := CreateAuthGroup(ctx, ag1, false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create group +globs", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}, {
					ChangeType: ChangeGroupGlobsAdded,
					Globs:      []string{"user:*@example.com"},
				}})

				// Add globs to already existing group
				ag1.Globs = append(ag1.Globs, "user:test-*@test.com")
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group +globs", 2, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupGlobsAdded,
					Globs:      []string{"user:test-*@test.com"},
				}})

				ag1.Globs = []string{"user:test-*@test.com"}
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				So(err, ShouldBeNil)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group -globs", 3, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupGlobsRemoved,
					Globs:      []string{"user:*@example.com"},
				}})

				So(DeleteAuthGroup(ctx, ag1.ID, "", false, "Go pRPC API", false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 8)
				actualChanges, err = generateChanges(ctx, 4, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete group -globs", 4, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupGlobsRemoved,
					Globs:      []string{"user:test-*@test.com"},
				}, {
					ChangeType: ChangeGroupDeleted,
					OldOwners:  AdminGroup,
				}})
			})

			Convey("AuthGroup add/remove nested", func() {
				ag2 := makeAuthGroup(ctx, "group-2")
				ag3 := makeAuthGroup(ctx, "group-3")
				So(datastore.Put(ctx, ag2, ag3), ShouldBeNil)

				// Add globs in create
				ag1 := makeAuthGroup(ctx, "group-1")
				ag1.Nested = []string{ag2.ID}
				_, err := CreateAuthGroup(ctx, ag1, false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create group +nested", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}, {
					ChangeType: ChangeGroupNestedAdded,
					Nested:     []string{"group-2"},
				}})

				// Add globs to already existing group
				ag1.Nested = append(ag1.Nested, ag3.ID)
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group +nested", 2, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupNestedAdded,
					Nested:     []string{"group-3"},
				}})

				ag1.Nested = []string{"group-2"}
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", false, "Go pRPC API", false)
				So(err, ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				So(err, ShouldBeNil)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update group -nested", 3, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupNestedRemoved,
					Nested:     []string{"group-3"},
				}})

				So(DeleteAuthGroup(ctx, ag1.ID, "", false, "Go pRPC API", false), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 8)
				actualChanges, err = generateChanges(ctx, 4, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete group -nested", 4, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupNestedRemoved,
					Nested:     []string{"group-2"},
				}, {
					ChangeType: ChangeGroupDeleted,
					OldOwners:  AdminGroup,
				}})
			})
		})

		Convey("AuthIPAllowlist changes", func() {
			Convey("AuthIPAllowlist Created/Deleted +/- subnets", func() {
				// Creation with no subnet
				baseSubnetMap := make(map[string][]string)
				baseSubnetMap["test-allowlist-1"] = []string{}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create allowlist", 1, actualChanges, []*AuthDBChange{{
					ChangeType:  ChangeIPALCreated,
					Description: "Imported from ip_allowlist.cfg",
				}})

				// Deletion with no subnet
				baseSubnetMap = map[string][]string{}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete allowlist", 2, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeIPALDeleted,
					OldDescription: "Imported from ip_allowlist.cfg",
				}})

				// Creation with subnets
				baseSubnetMap["test-allowlist-1"] = []string{"123.4.5.6"}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 6)
				actualChanges, err = generateChanges(ctx, 3, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "create allowlist w/ subnet", 3, actualChanges, []*AuthDBChange{{
					ChangeType:  ChangeIPALCreated,
					Description: "Imported from ip_allowlist.cfg",
				}, {
					ChangeType: ChangeIPALSubnetsAdded,
					Subnets:    []string{"123.4.5.6"},
				}})

				// Add subnet
				baseSubnetMap["test-allowlist-1"] = append(baseSubnetMap["test-allowlist-1"], "567.8.9.10")
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 8)
				actualChanges, err = generateChanges(ctx, 4, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "add subnet", 4, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeIPALSubnetsAdded,
					Subnets:    []string{"567.8.9.10"},
				}})

				// Remove subnet
				baseSubnetMap["test-allowlist-1"] = baseSubnetMap["test-allowlist-1"][1:]
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 10)
				actualChanges, err = generateChanges(ctx, 5, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "remove subnet", 5, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeIPALSubnetsRemoved,
					Subnets:    []string{"123.4.5.6"},
				}})

				// Delete allowlist with subnet
				baseSubnetMap = map[string][]string{}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 12)
				actualChanges, err = generateChanges(ctx, 6, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "delete allowlist w/ subnet", 6, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeIPALDeleted,
					OldDescription: "Imported from ip_allowlist.cfg",
				}, {
					ChangeType: ChangeIPALSubnetsRemoved,
					Subnets:    []string{"567.8.9.10"},
				}})
			})

			Convey("AuthIPAllowlist description changed", func() {
				baseSubnetMap := make(map[string][]string)
				baseSubnetMap["test-allowlist-1"] = []string{}
				So(UpdateAllowlistEntities(ctx, baseSubnetMap, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				_, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)

				al, err := GetAuthIPAllowlist(ctx, "test-allowlist-1")
				So(err, ShouldBeNil)
				So(runAuthDBChange(ctx, "test for AuthIPAllowlist changelog", func(ctx context.Context, cae commitAuthEntity) error {
					al.Description = "new-desc"
					return cae(al, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), false)
				}), ShouldBeNil)
				actualChanges, err := generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "change description", 2, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeIPALDescriptionChanged,
					Description:    "new-desc",
					OldDescription: "Imported from ip_allowlist.cfg",
				}})
			})
		})

		Convey("AuthGlobalConfig changes", func() {
			Convey("AuthGlobalConfig ClientID/ClientSecret mismatch", func() {
				baseCfg := &configspb.OAuthConfig{
					PrimaryClientId:     "test-client-id",
					PrimaryClientSecret: "test-client-secret",
				}

				// Old doesn't exist yet
				So(UpdateAuthGlobalConfig(ctx, baseCfg, nil, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with no old config present", 1, actualChanges, []*AuthDBChange{{
					ChangeType:        ChangeConfOauthClientChanged,
					OauthClientID:     "test-client-id",
					OauthClientSecret: "test-client-secret",
				}})

				newCfg := &configspb.OAuthConfig{
					PrimaryClientId: "diff-client-id",
				}
				So(UpdateAuthGlobalConfig(ctx, newCfg, nil, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with client id changed", 2, actualChanges, []*AuthDBChange{{
					ChangeType:    ChangeConfOauthClientChanged,
					OauthClientID: "diff-client-id",
				}})
			})

			Convey("AuthGlobalConfig additional +/- ClientID's", func() {
				baseCfg := &configspb.OAuthConfig{
					ClientIds: []string{"test.example.com"},
				}

				So(UpdateAuthGlobalConfig(ctx, baseCfg, nil, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with client ids, old config not present", 1, actualChanges, []*AuthDBChange{{
					ChangeType:               ChangeConfClientIDsAdded,
					OauthAdditionalClientIDs: []string{"test.example.com"},
				}})

				newCfg := &configspb.OAuthConfig{
					ClientIds: []string{"not-test.example.com"},
				}
				So(UpdateAuthGlobalConfig(ctx, newCfg, nil, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with client id added and client id removed, old config is present",
					2, actualChanges, []*AuthDBChange{{
						ChangeType:               ChangeConfClientIDsAdded,
						OauthAdditionalClientIDs: []string{"not-test.example.com"},
					}, {
						ChangeType:               ChangeConfClientIDsRemoved,
						OauthAdditionalClientIDs: []string{"test.example.com"},
					}})
			})

			Convey("AuthGlobalConfig TokenServerURL change", func() {
				baseCfg := &configspb.OAuthConfig{
					TokenServerUrl: "test-token-server-url.example.com",
				}

				So(UpdateAuthGlobalConfig(ctx, baseCfg, nil, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with token server url, old config not present", 1, actualChanges, []*AuthDBChange{{
					ChangeType:        ChangeConfTokenServerURLChanged,
					TokenServerURLNew: "test-token-server-url.example.com",
				}})
			})

			Convey("AuthGlobalConfig Security Config change", func() {
				baseCfg := &configspb.OAuthConfig{}
				secCfg := &protocol.SecurityConfig{
					InternalServiceRegexp: []string{"abc"},
				}
				So(UpdateAuthGlobalConfig(ctx, baseCfg, secCfg, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				expectedNewConfig, _ := proto.Marshal(secCfg)
				validateChanges(ctx, "update config with security config, old config not present", 1, actualChanges, []*AuthDBChange{{
					ChangeType:        ChangeConfSecurityConfigChanged,
					SecurityConfigNew: expectedNewConfig,
				}})

				secCfg = &protocol.SecurityConfig{
					InternalServiceRegexp: []string{"def"},
				}
				expectedOldConfig := expectedNewConfig
				expectedNewConfig, _ = proto.Marshal(secCfg)
				So(UpdateAuthGlobalConfig(ctx, baseCfg, secCfg, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 4)
				actualChanges, err = generateChanges(ctx, 2, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "update config with security config, old config present", 2, actualChanges, []*AuthDBChange{{
					ChangeType:        ChangeConfSecurityConfigChanged,
					SecurityConfigNew: expectedNewConfig,
					SecurityConfigOld: expectedOldConfig,
				}})
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
				expectedNewConfig, _ := proto.Marshal(secCfg)
				So(UpdateAuthGlobalConfig(ctx, baseCfg, secCfg, false, "Go pRPC API"), ShouldBeNil)
				So(taskScheduler.Tasks(), ShouldHaveLength, 2)
				actualChanges, err := generateChanges(ctx, 1, false)
				So(err, ShouldBeNil)
				validateChanges(ctx, "all changes at once, old config not present", 1, actualChanges, []*AuthDBChange{{
					ChangeType:        ChangeConfOauthClientChanged,
					OauthClientID:     "test-client-id",
					OauthClientSecret: "test-client-secret",
				}, {
					ChangeType:               ChangeConfClientIDsAdded,
					OauthAdditionalClientIDs: []string{"a", "b", "c"},
				}, {
					ChangeType:        ChangeConfTokenServerURLChanged,
					TokenServerURLNew: "token-server.example.com",
				}, {
					ChangeType:        ChangeConfSecurityConfigChanged,
					SecurityConfigNew: expectedNewConfig,
				}})
			})
		})

		Convey("AuthRealmsGlobals changes", func() {
			permCfg := &configspb.PermissionsConfig{
				Role: []*configspb.PermissionsConfig_Role{
					{
						Name: "role/test.role.editor",
						Permissions: []*protocol.Permission{
							{
								Name: "test.perm.edit",
							},
						},
					},
					{
						Name: "role/test.role.creator",
						Permissions: []*protocol.Permission{
							{
								Name: "test.perm.create",
							},
						},
					},
				},
			}
			So(UpdateAuthRealmsGlobals(ctx, permCfg, false, "Go pRPC API"), ShouldBeNil)
			So(taskScheduler.Tasks(), ShouldHaveLength, 2)
			actualChanges, err := generateChanges(ctx, 1, false)
			So(err, ShouldBeNil)
			validateChanges(ctx, "update realms globals, old config not present", 1, actualChanges, []*AuthDBChange{{
				ChangeType:       ChangeRealmsGlobalsChanged,
				PermissionsAdded: []string{"test.perm.create", "test.perm.edit"},
			}})

			permCfg = &configspb.PermissionsConfig{
				Role: []*configspb.PermissionsConfig_Role{
					{
						Name: "role/test.role.creator",
						Permissions: []*protocol.Permission{
							{
								Name:     "test.perm.create",
								Internal: true,
							},
						},
					},
				},
			}
			So(UpdateAuthRealmsGlobals(ctx, permCfg, false, "Go pRPC API"), ShouldBeNil)
			So(taskScheduler.Tasks(), ShouldHaveLength, 4)
			actualChanges, err = generateChanges(ctx, 2, false)
			So(err, ShouldBeNil)
			validateChanges(ctx, "update realms globals, old config present", 2, actualChanges, []*AuthDBChange{{
				ChangeType:         ChangeRealmsGlobalsChanged,
				PermissionsChanged: []string{"test.perm.create"},
				PermissionsRemoved: []string{"test.perm.edit"},
			}})
		})

		Convey("Changelog generation cascades", func() {
			// Create and add 3 AuthGroups.
			groupNames := []string{"group-1", "group-2", "group-3"}
			for _, groupName := range groupNames {
				ag := makeAuthGroup(ctx, groupName)
				_, err := CreateAuthGroup(ctx, ag, false, "Go pRPC API", false)
				So(err, ShouldBeNil)
			}
			// There should be a changelog task and replication task for
			// each group.
			So(taskScheduler.Tasks(), ShouldHaveLength, 6)

			actualChanges, err := generateChanges(ctx, 1, false)
			So(err, ShouldBeNil)
			validateChanges(ctx, "created first group in cascade test", 1, actualChanges, []*AuthDBChange{{
				ChangeType: ChangeGroupCreated,
				Owners:     AdminGroup,
			}})
			// First change has no prior revision, so a task should not
			// have been added.
			So(taskScheduler.Tasks(), ShouldHaveLength, 6)

			actualChanges, err = generateChanges(ctx, 3, false)
			So(err, ShouldBeNil)
			validateChanges(ctx, "created third group in cascade test", 3, actualChanges, []*AuthDBChange{{
				ChangeType: ChangeGroupCreated,
				Owners:     AdminGroup,
			}})
			// Changelog for rev 2 has not been generated yet, so there
			// should be an added task.
			So(taskScheduler.Tasks(), ShouldHaveLength, 7)

			actualChanges, err = generateChanges(ctx, 2, false)
			So(err, ShouldBeNil)
			validateChanges(ctx, "created second group in cascade test", 2, actualChanges, []*AuthDBChange{{
				ChangeType: ChangeGroupCreated,
				Owners:     AdminGroup,
			}})
			// Changelog for rev 1 has already been generated, so a task
			// should not have been added.
			So(taskScheduler.Tasks(), ShouldHaveLength, 7)
		})

		Convey("dry run of changelog generation works", func() {
			agDryRun := makeAuthGroup(ctx, "group-dry-run")
			_, err := CreateAuthGroup(ctx, agDryRun, false, "Go pRPC API", false)
			So(err, ShouldBeNil)
			So(taskScheduler.Tasks(), ShouldHaveLength, 2)
			_, err = generateChanges(ctx, 1, true)
			So(err, ShouldBeNil)

			// Check the changelog was created in dry run mode.
			actualChanges := getChanges(ctx, 1, true)
			So(actualChanges, ShouldHaveLength, 1)
			So(actualChanges[0].Kind, ShouldEqual, "V2AuthDBChange")
			So(actualChanges[0].Parent, ShouldEqual, ChangeLogRevisionKey(ctx, 1, true))
			So(actualChanges[0].ChangeType, ShouldEqual, ChangeGroupCreated)
			So(actualChanges[0].Owners, ShouldEqual, AdminGroup)

			// Check there were no changes written with dry run mode off.
			So(getChanges(ctx, 1, false), ShouldBeEmpty)
		})
	})
}

func TestPropertyMapHelpers(t *testing.T) {
	Convey("getStringSliceProp robustly returns strings", t, func() {
		pm := datastore.PropertyMap{}
		pm["testField"] = datastore.PropertySlice{
			datastore.MkPropertyNI([]byte("test data first entry")),
			datastore.MkPropertyNI("test data second entry"),
		}

		So(getStringSliceProp(pm, "testField"), ShouldEqual, []string{
			"test data first entry",
			"test data second entry",
		})
	})
}
