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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/testsupport"
)

func testAuthDBGroupChange(ctx context.Context, t testing.TB, target string, changeType ChangeType, authDBRev int64) *AuthDBChange {
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
	assert.Loosely(t, err, should.BeNil)
	return change
}

func testAuthDBIPAllowlistChange(ctx context.Context, t testing.TB, authDBRev int64) *AuthDBChange {
	change := &AuthDBChange{
		Kind:           "AuthDBChange",
		Parent:         ChangeLogRevisionKey(ctx, authDBRev),
		Class:          []string{"AuthDBChange", "AuthDBIPWhitelistChange"},
		ChangeType:     3000,
		Comment:        "comment",
		Description:    "description",
		OldDescription: "",
		Target:         "AuthIPWhitelist$a",
		AuthDBRev:      authDBRev,
		When:           time.Date(2021, time.December, 12, 1, 0, 0, 0, time.UTC),
		Who:            "user:admin@example.com",
		AppVersion:     "123-45abc",
	}

	var err error
	change.ID, err = ChangeID(ctx, change)
	assert.Loosely(t, err, should.BeNil)
	return change
}

func testAuthDBIPAllowlistAssignmentChange(ctx context.Context, t testing.TB, authDBRev int64) *AuthDBChange {
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
	assert.Loosely(t, err, should.BeNil)
	return change
}

func testAuthDBConfigChange(ctx context.Context, t testing.TB, authDBRev int64) *AuthDBChange {
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
		Who:               "user:admin@example.com",
		AppVersion:        "123-45abc",
	}

	var err error
	change.ID, err = ChangeID(ctx, change)
	assert.Loosely(t, err, should.BeNil)
	return change
}

func testAuthRealmsGlobalsChange(ctx context.Context, t testing.TB, authDBRev int64) *AuthDBChange {
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
	assert.Loosely(t, err, should.BeNil)
	return change
}

func testAuthProjectRealmsChange(ctx context.Context, t testing.TB, authDBRev int64) *AuthDBChange {
	change := &AuthDBChange{
		Kind:           "AuthDBChange",
		Parent:         ChangeLogRevisionKey(ctx, authDBRev),
		Class:          []string{"AuthDBChange", "AuthProjectRealmsChange"},
		ChangeType:     10200,
		Comment:        "comment",
		ConfigRevOld:   "",
		ConfigRevNew:   "",
		PermsRevOld:    "auth_service_ver:100-00abc",
		PermsRevNew:    "auth_service_ver:123-45abc",
		ProjectsRevOld: "projects.cfg:old",
		ProjectsRevNew: "projects.cfg:new",
		Target:         "AuthProjectRealms$repo",
		When:           time.Date(2019, time.January, 11, 1, 0, 0, 0, time.UTC),
		Who:            "user:test@example.com",
		AppVersion:     "123-45abc",
	}

	var err error
	change.ID, err = ChangeID(ctx, change)
	assert.Loosely(t, err, should.BeNil)
	return change
}

func svcRev(permsRev, projsRev string) ServiceCfgRev {
	return ServiceCfgRev{
		PermsRev:    permsRev,
		ProjectsRev: projsRev,
	}
}

////////////////////////////////////////////////////////////////////////////////

func TestGetAllAuthDBChange(t *testing.T) {
	t.Parallel()
	ftt.Run("Testing GetAllAuthDBChange", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		assert.Loosely(t, datastore.Put(ctx,
			testAuthDBGroupChange(ctx, t, "AuthGroup$groupA", ChangeGroupCreated, 1120),
			testAuthDBGroupChange(ctx, t, "AuthGroup$groupB", ChangeGroupMembersAdded, 1120),
			testAuthDBGroupChange(ctx, t, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
			testAuthDBGroupChange(ctx, t, "AuthGroup$groupC", ChangeGroupMembersAdded, 1135),
			testAuthDBGroupChange(ctx, t, "AuthGroup$groupC", ChangeGroupOwnersChanged, 1110),
			testAuthDBIPAllowlistChange(ctx, t, 1120),
			testAuthDBIPAllowlistAssignmentChange(ctx, t, 1121),
			testAuthDBConfigChange(ctx, t, 1115),
			testAuthRealmsGlobalsChange(ctx, t, 1120),
			testAuthProjectRealmsChange(ctx, t, 1115),
		), should.BeNil)

		t.Run("Sort by key", func(t *ftt.Test) {
			req := &rpcpb.ListChangeLogsRequest{
				PageSize: 5,
			}
			changes, pageToken, err := GetAllAuthDBChange(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pageToken, should.NotBeEmpty)
			assert.Loosely(t, changes, should.Match([]*AuthDBChange{
				testAuthDBGroupChange(ctx, t, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
				testAuthDBGroupChange(ctx, t, "AuthGroup$groupC", ChangeGroupMembersAdded, 1135),
				testAuthDBIPAllowlistAssignmentChange(ctx, t, 1121),
				testAuthRealmsGlobalsChange(ctx, t, 1120),
				testAuthDBIPAllowlistChange(ctx, t, 1120),
			}))

			req.PageToken = pageToken
			changes, _, err = GetAllAuthDBChange(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, changes, should.Match([]*AuthDBChange{
				testAuthDBGroupChange(ctx, t, "AuthGroup$groupB", ChangeGroupMembersAdded, 1120),
				testAuthDBGroupChange(ctx, t, "AuthGroup$groupA", ChangeGroupCreated, 1120),
				testAuthProjectRealmsChange(ctx, t, 1115),
				testAuthDBConfigChange(ctx, t, 1115),
				testAuthDBGroupChange(ctx, t, "AuthGroup$groupC", ChangeGroupOwnersChanged, 1110),
			}))
		})
		t.Run("Filter by Target", func(t *ftt.Test) {
			req := &rpcpb.ListChangeLogsRequest{
				Target:   "AuthGroup$groupB",
				PageSize: 10,
			}
			changes, _, err := GetAllAuthDBChange(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, changes, should.Match([]*AuthDBChange{
				testAuthDBGroupChange(ctx, t, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
				testAuthDBGroupChange(ctx, t, "AuthGroup$groupB", ChangeGroupMembersAdded, 1120),
			}))
		})
		t.Run("Filter by AuthDBRev", func(t *ftt.Test) {
			req := &rpcpb.ListChangeLogsRequest{
				AuthDbRev: 1136,
				PageSize:  10,
			}
			changes, _, err := GetAllAuthDBChange(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, changes, should.Match([]*AuthDBChange{
				testAuthDBGroupChange(ctx, t, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
			}))
		})
		t.Run("Filter by Modifier", func(t *ftt.Test) {
			req := &rpcpb.ListChangeLogsRequest{
				Modifier: "user:admin@example.com",
				PageSize: 10,
			}
			changes, _, err := GetAllAuthDBChange(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, changes, should.Match([]*AuthDBChange{
				testAuthDBIPAllowlistChange(ctx, t, 1120),
				testAuthDBConfigChange(ctx, t, 1115),
			}))
		})
		t.Run("Filter by Modifier and Target", func(t *ftt.Test) {
			req := &rpcpb.ListChangeLogsRequest{
				Target:   "AuthGlobalConfig$test",
				Modifier: "user:admin@example.com",
				PageSize: 10,
			}
			changes, _, err := GetAllAuthDBChange(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, changes, should.Match([]*AuthDBChange{
				testAuthDBConfigChange(ctx, t, 1115),
			}))
		})
		t.Run("Return error when target is invalid", func(t *ftt.Test) {
			req := &rpcpb.ListChangeLogsRequest{
				Target:   "groupname",
				PageSize: 10,
			}
			_, _, err := GetAllAuthDBChange(ctx, req)
			assert.Loosely(t, err, should.ErrLike("Invalid change log target \"groupname\""))
		})
	})
}

func TestGenerateChanges(t *testing.T) {
	t.Parallel()

	ftt.Run("GenerateChanges", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{AdminGroup},
		})
		ctx = testsupport.SetTestContextSigner(ctx, "test-app-id", "test-app-id@example.com")
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		assert.Loosely(t, datastore.Put(ctx, testAuthGroup(ctx, AdminGroup)), should.BeNil)

		//////////////////////////////////////////////////////////
		// Helper functions
		getChanges := func(ctx context.Context, authDBRev int64) []*AuthDBChange {
			ancestor := constructLogRevisionKey(ctx, authDBRev)
			query := datastore.NewQuery("AuthDBChange").Ancestor(ancestor)
			changes := []*AuthDBChange{}
			err := datastore.Run(ctx, query, func(change *AuthDBChange) {
				changes = append(changes, change)
			})
			assert.Loosely(t, err, should.BeNil)
			return changes
		}

		// The fields in AuthDBChange to ignore when comparing results;
		// these fields are not signficant to the test cases below.
		ignoredAuthDBChangeFields := cmpopts.IgnoreFields(AuthDBChange{},
			"Kind", "ID", "Parent", "Class", "Target", "Who", "When", "Comment", "AppVersion")

		validateChanges := func(ctx context.Context, msg string, authDBRev int64, actualChanges []*AuthDBChange, expectedChanges []*AuthDBChange) {
			changeCount := len(expectedChanges)
			if !check.Loosely(t, actualChanges, should.HaveLength(changeCount)) {
				t.Fatal(msg)
			}

			// Check each actual and exxpected changes are similar.
			for i := range changeCount {
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
			if !check.Loosely(t, getChanges(ctx, authDBRev), should.Match(actualChanges)) {
				t.Fatal(msg)
			}
		}
		//////////////////////////////////////////////////////////

		t.Run("AuthGroup changes", func(t *ftt.Test) {
			t.Run("AuthGroup Created/Deleted", func(t *ftt.Test) {
				ag1 := makeAuthGroup(ctx, "group-1")
				_, err := CreateAuthGroup(ctx, ag1, "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				// Check that common fields were set as expected; these fields
				// are ignored for most other test cases.
				assert.Loosely(t, actualChanges, should.Match([]*AuthDBChange{{
					ID:         "AuthGroup$group-1!1000",
					Class:      []string{"AuthDBChange", "AuthDBGroupChange"},
					ChangeType: ChangeGroupCreated,
					Target:     "AuthGroup$group-1",
					Kind:       "AuthDBChange",
					Parent:     ChangeLogRevisionKey(ctx, 1),
					AuthDBRev:  1,
					Who:        "user:someone@example.com",
					When:       testCreatedTS,
					Comment:    "Go pRPC API",
					AppVersion: "test-version",
					Owners:     AdminGroup,
				}}))
				validateChanges(ctx, "create group", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}})

				assert.Loosely(t, DeleteAuthGroup(ctx, ag1.ID, "", "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				actualChanges, err = generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "delete group", 2, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupDeleted,
					OldOwners:  AdminGroup,
				}})

				// Check calling generateChanges for an already-processed
				// AuthDB revision does not make duplicate changes.
				repeated, err := generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, repeated, should.BeNil)
				// Check the changelog for the revision actually exists.
				expectedStoredChanges := []*AuthDBChange(actualChanges)
				sort.Slice(expectedStoredChanges, func(i, j int) bool {
					return expectedStoredChanges[i].ChangeType < expectedStoredChanges[j].ChangeType
				})
				assert.Loosely(t, getChanges(ctx, 2), should.Match(expectedStoredChanges))
			})

			t.Run("AuthGroup Owners / Description changed", func(t *ftt.Test) {
				og := makeAuthGroup(ctx, "owning-group")
				assert.Loosely(t, datastore.Put(ctx, og), should.BeNil)

				ag1 := makeAuthGroup(ctx, "group-1")
				_, err := CreateAuthGroup(ctx, ag1, "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "create group no owners", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}})

				ag1.Owners = og.ID
				ag1.Description = "test-desc"
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				actualChanges, err = generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update group owners & desc", 2, actualChanges, []*AuthDBChange{{
					ChangeType:  ChangeGroupDescriptionChanged,
					Description: "test-desc",
				}, {
					ChangeType: ChangeGroupOwnersChanged,
					Owners:     "owning-group",
					OldOwners:  AdminGroup,
				}})

				ag1.Description = "new-desc"
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(6))
				actualChanges, err = generateChanges(ctx, 3)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update group desc", 3, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeGroupDescriptionChanged,
					Description:    "new-desc",
					OldDescription: "test-desc",
				}})
			})

			t.Run("AuthGroup add/remove Members", func(t *ftt.Test) {
				// Add members in Create
				ag1 := makeAuthGroup(ctx, "group-1")
				ag1.Members = []string{"user:someone@example.com"}
				_, err := CreateAuthGroup(ctx, ag1, "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "create group +mems", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}, {
					ChangeType: ChangeGroupMembersAdded,
					Members:    []string{"user:someone@example.com"},
				}})

				// Add members to already existing group
				ag1.Members = append(ag1.Members, "user:another-one@example.com")
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				actualChanges, err = generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update group +mems", 2, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupMembersAdded,
					Members:    []string{"user:another-one@example.com"},
				}})

				// Remove members from existing group
				ag1.Members = []string{"user:someone@example.com"}
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(6))
				actualChanges, err = generateChanges(ctx, 3)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update group -mems", 3, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupMembersRemoved,
					Members:    []string{"user:another-one@example.com"},
				}})

				// Remove members when deleting group
				assert.Loosely(t, DeleteAuthGroup(ctx, ag1.ID, "", "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(8))
				actualChanges, err = generateChanges(ctx, 4)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "delete group -mems", 4, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupMembersRemoved,
					Members:    []string{"user:someone@example.com"},
				}, {
					ChangeType: ChangeGroupDeleted,
					OldOwners:  AdminGroup,
				}})
			})

			t.Run("AuthGroup add/remove globs", func(t *ftt.Test) {
				// Add globs in create
				ag1 := makeAuthGroup(ctx, "group-1")
				ag1.Globs = []string{"user:*@example.com"}
				_, err := CreateAuthGroup(ctx, ag1, "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "create group +globs", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}, {
					ChangeType: ChangeGroupGlobsAdded,
					Globs:      []string{"user:*@example.com"},
				}})

				// Add globs to already existing group
				ag1.Globs = append(ag1.Globs, "user:test-*@test.com")
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				actualChanges, err = generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update group +globs", 2, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupGlobsAdded,
					Globs:      []string{"user:test-*@test.com"},
				}})

				ag1.Globs = []string{"user:test-*@test.com"}
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(6))
				assert.Loosely(t, err, should.BeNil)
				actualChanges, err = generateChanges(ctx, 3)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update group -globs", 3, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupGlobsRemoved,
					Globs:      []string{"user:*@example.com"},
				}})

				assert.Loosely(t, DeleteAuthGroup(ctx, ag1.ID, "", "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(8))
				actualChanges, err = generateChanges(ctx, 4)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "delete group -globs", 4, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupGlobsRemoved,
					Globs:      []string{"user:test-*@test.com"},
				}, {
					ChangeType: ChangeGroupDeleted,
					OldOwners:  AdminGroup,
				}})
			})

			t.Run("AuthGroup add/remove nested", func(t *ftt.Test) {
				ag2 := makeAuthGroup(ctx, "group-2")
				ag3 := makeAuthGroup(ctx, "group-3")
				assert.Loosely(t, datastore.Put(ctx, ag2, ag3), should.BeNil)

				// Add globs in create
				ag1 := makeAuthGroup(ctx, "group-1")
				ag1.Nested = []string{ag2.ID}
				_, err := CreateAuthGroup(ctx, ag1, "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "create group +nested", 1, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupCreated,
					Owners:     AdminGroup,
				}, {
					ChangeType: ChangeGroupNestedAdded,
					Nested:     []string{"group-2"},
				}})

				// Add globs to already existing group
				ag1.Nested = append(ag1.Nested, ag3.ID)
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				actualChanges, err = generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update group +nested", 2, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupNestedAdded,
					Nested:     []string{"group-3"},
				}})

				ag1.Nested = []string{"group-2"}
				_, err = UpdateAuthGroup(ctx, ag1, nil, "", "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(6))
				assert.Loosely(t, err, should.BeNil)
				actualChanges, err = generateChanges(ctx, 3)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update group -nested", 3, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupNestedRemoved,
					Nested:     []string{"group-3"},
				}})

				assert.Loosely(t, DeleteAuthGroup(ctx, ag1.ID, "", "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(8))
				actualChanges, err = generateChanges(ctx, 4)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "delete group -nested", 4, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeGroupNestedRemoved,
					Nested:     []string{"group-2"},
				}, {
					ChangeType: ChangeGroupDeleted,
					OldOwners:  AdminGroup,
				}})
			})
		})

		t.Run("AuthIPAllowlist changes", func(t *ftt.Test) {
			t.Run("AuthIPAllowlist Created/Deleted +/- subnets", func(t *ftt.Test) {
				// Creation with no subnet
				baseSubnetMap := make(map[string][]string)
				baseSubnetMap["test-allowlist-1"] = []string{}
				assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "create allowlist", 1, actualChanges, []*AuthDBChange{{
					ChangeType:  ChangeIPALCreated,
					Description: "Imported from ip_allowlist.cfg",
				}})

				// Deletion with no subnet
				baseSubnetMap = map[string][]string{}
				assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				actualChanges, err = generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "delete allowlist", 2, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeIPALDeleted,
					OldDescription: "Imported from ip_allowlist.cfg",
				}})

				// Creation with subnets
				baseSubnetMap["test-allowlist-1"] = []string{"123.4.5.6"}
				assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(6))
				actualChanges, err = generateChanges(ctx, 3)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "create allowlist w/ subnet", 3, actualChanges, []*AuthDBChange{{
					ChangeType:  ChangeIPALCreated,
					Description: "Imported from ip_allowlist.cfg",
				}, {
					ChangeType: ChangeIPALSubnetsAdded,
					Subnets:    []string{"123.4.5.6"},
				}})

				// Add subnet
				baseSubnetMap["test-allowlist-1"] = append(baseSubnetMap["test-allowlist-1"], "567.8.9.10")
				assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(8))
				actualChanges, err = generateChanges(ctx, 4)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "add subnet", 4, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeIPALSubnetsAdded,
					Subnets:    []string{"567.8.9.10"},
				}})

				// Remove subnet
				baseSubnetMap["test-allowlist-1"] = baseSubnetMap["test-allowlist-1"][1:]
				assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(10))
				actualChanges, err = generateChanges(ctx, 5)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "remove subnet", 5, actualChanges, []*AuthDBChange{{
					ChangeType: ChangeIPALSubnetsRemoved,
					Subnets:    []string{"123.4.5.6"},
				}})

				// Delete allowlist with subnet
				baseSubnetMap = map[string][]string{}
				assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(12))
				actualChanges, err = generateChanges(ctx, 6)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "delete allowlist w/ subnet", 6, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeIPALDeleted,
					OldDescription: "Imported from ip_allowlist.cfg",
				}, {
					ChangeType: ChangeIPALSubnetsRemoved,
					Subnets:    []string{"567.8.9.10"},
				}})
			})

			t.Run("AuthIPAllowlist description changed", func(t *ftt.Test) {
				baseSubnetMap := make(map[string][]string)
				baseSubnetMap["test-allowlist-1"] = []string{}
				assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				_, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)

				al, err := GetAuthIPAllowlist(ctx, "test-allowlist-1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, runAuthDBChange(ctx, "test for AuthIPAllowlist changelog", func(ctx context.Context, cae commitAuthEntity) error {
					al.Description = "new-desc"
					return cae(al, clock.Now(ctx).UTC(), auth.CurrentIdentity(ctx), false)
				}), should.BeNil)
				actualChanges, err := generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "change description", 2, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeIPALDescriptionChanged,
					Description:    "new-desc",
					OldDescription: "Imported from ip_allowlist.cfg",
				}})
			})
		})

		t.Run("AuthGlobalConfig changes", func(t *ftt.Test) {
			t.Run("AuthGlobalConfig ClientID/ClientSecret mismatch", func(t *ftt.Test) {
				baseCfg := &configspb.OAuthConfig{
					PrimaryClientId:     "test-client-id",
					PrimaryClientSecret: "test-client-secret",
				}

				// Old doesn't exist yet
				assert.Loosely(t, updateAuthGlobalConfig(ctx, baseCfg, nil, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update config with no old config present", 1, actualChanges, []*AuthDBChange{{
					ChangeType:        ChangeConfOauthClientChanged,
					OauthClientID:     "test-client-id",
					OauthClientSecret: "test-client-secret",
				}})

				newCfg := &configspb.OAuthConfig{
					PrimaryClientId: "diff-client-id",
				}
				assert.Loosely(t, updateAuthGlobalConfig(ctx, newCfg, nil, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				actualChanges, err = generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update config with client id changed", 2, actualChanges, []*AuthDBChange{{
					ChangeType:    ChangeConfOauthClientChanged,
					OauthClientID: "diff-client-id",
				}})
			})

			t.Run("AuthGlobalConfig additional +/- ClientID's", func(t *ftt.Test) {
				baseCfg := &configspb.OAuthConfig{
					ClientIds: []string{"test.example.com"},
				}

				assert.Loosely(t, updateAuthGlobalConfig(ctx, baseCfg, nil, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update config with client ids, old config not present", 1, actualChanges, []*AuthDBChange{{
					ChangeType:               ChangeConfClientIDsAdded,
					OauthAdditionalClientIDs: []string{"test.example.com"},
				}})

				newCfg := &configspb.OAuthConfig{
					ClientIds: []string{"not-test.example.com"},
				}
				assert.Loosely(t, updateAuthGlobalConfig(ctx, newCfg, nil, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				actualChanges, err = generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update config with client id added and client id removed, old config is present",
					2, actualChanges, []*AuthDBChange{{
						ChangeType:               ChangeConfClientIDsAdded,
						OauthAdditionalClientIDs: []string{"not-test.example.com"},
					}, {
						ChangeType:               ChangeConfClientIDsRemoved,
						OauthAdditionalClientIDs: []string{"test.example.com"},
					}})
			})

			t.Run("AuthGlobalConfig TokenServerURL change", func(t *ftt.Test) {
				baseCfg := &configspb.OAuthConfig{
					TokenServerUrl: "test-token-server-url.example.com",
				}

				assert.Loosely(t, updateAuthGlobalConfig(ctx, baseCfg, nil, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update config with token server url, old config not present", 1, actualChanges, []*AuthDBChange{{
					ChangeType:        ChangeConfTokenServerURLChanged,
					TokenServerURLNew: "test-token-server-url.example.com",
				}})
			})

			t.Run("AuthGlobalConfig Security Config change", func(t *ftt.Test) {
				baseCfg := &configspb.OAuthConfig{}
				secCfg := &protocol.SecurityConfig{
					InternalServiceRegexp: []string{"abc"},
				}
				assert.Loosely(t, updateAuthGlobalConfig(ctx, baseCfg, secCfg, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
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
				assert.Loosely(t, updateAuthGlobalConfig(ctx, baseCfg, secCfg, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
				actualChanges, err = generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "update config with security config, old config present", 2, actualChanges, []*AuthDBChange{{
					ChangeType:        ChangeConfSecurityConfigChanged,
					SecurityConfigNew: expectedNewConfig,
					SecurityConfigOld: expectedOldConfig,
				}})
			})

			t.Run("AuthGlobalConfig all changes at once", func(t *ftt.Test) {
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
				assert.Loosely(t, updateAuthGlobalConfig(ctx, baseCfg, secCfg, "Go pRPC API"), should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
				actualChanges, err := generateChanges(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
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

		t.Run("AuthProjectRealms changes", func(t *ftt.Test) {
			proj1Realms := &protocol.Realms{
				Permissions: makeTestPermissions("luci.dev.p2", "luci.dev.z", "luci.dev.p1"),
				Realms: []*protocol.Realm{
					{
						Name: "proj1:@root",
						Bindings: []*protocol.Binding{
							{
								// Permissions p2, z, p1.
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr1"},
							},
						},
					},
				},
			}

			// Set up existing project realms.
			expandedRealms := []*ExpandedRealms{
				{
					CfgRev: &RealmsCfgRev{
						ProjectID:    "proj1",
						ConfigRev:    "10001",
						ConfigDigest: "test config digest",
					},
					Realms: proj1Realms,
				},
			}
			err := updateAuthProjectRealms(ctx, expandedRealms, svcRev("perms:abc", "proj:def"), "Go pRPC API")
			assert.Loosely(t, err, should.BeNil)

			t.Run("project realms created", func(t *ftt.Test) {
				expandedRealms := []*ExpandedRealms{
					{
						CfgRev: &RealmsCfgRev{
							ProjectID:    "proj2",
							ConfigRev:    "1",
							ConfigDigest: "test config digest",
						},
					},
				}

				err := updateAuthProjectRealms(ctx, expandedRealms, svcRev("perms:abc", "proj:def"), "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)

				actualChanges, err := generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "project realms created", 2, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeProjectRealmsCreated,
					ConfigRevNew:   "1",
					PermsRevNew:    "perms:abc",
					ProjectsRevNew: "proj:def",
				}})
			})

			t.Run("project realms deleted", func(t *ftt.Test) {
				err = deleteAuthProjectRealms(ctx, "proj1", "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)

				actualChanges, err := generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "project realms removed", 2, actualChanges, []*AuthDBChange{{
					ChangeType:     ChangeProjectRealmsRemoved,
					ConfigRevOld:   "10001",
					PermsRevOld:    "perms:abc",
					ProjectsRevOld: "proj:def",
				}})
			})

			t.Run("project config superficially changed", func(t *ftt.Test) {
				// New config revision, but the resulting realms are identical.
				updatedExpandedRealms := []*ExpandedRealms{
					{
						CfgRev: &RealmsCfgRev{
							ProjectID:    "proj1",
							ConfigRev:    "10002",
							ConfigDigest: "test config digest",
						},
						Realms: proj1Realms,
					},
				}
				err = updateAuthProjectRealms(ctx, updatedExpandedRealms, svcRev("perms:abc", "proj:def"), "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)

				actualChanges, err := generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actualChanges, should.BeEmpty)
			})

			t.Run("project config revision changed", func(t *ftt.Test) {
				updatedExpandedRealms := []*ExpandedRealms{
					{
						CfgRev: &RealmsCfgRev{
							ProjectID:    "proj1",
							ConfigRev:    "10002",
							ConfigDigest: "test config digest",
						},
						Realms: nil,
					},
				}
				err = updateAuthProjectRealms(ctx, updatedExpandedRealms, svcRev("perms:abc", "proj:def"), "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)

				actualChanges, err := generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "project realms changed", 2, actualChanges, []*AuthDBChange{{
					ChangeType:   ChangeProjectRealmsChanged,
					ConfigRevOld: "10001",
					ConfigRevNew: "10002",
				}})
			})

			t.Run("realms changed but revisions identical", func(t *ftt.Test) {
				updatedExpandedRealms := []*ExpandedRealms{
					{
						CfgRev: &RealmsCfgRev{
							ProjectID:    "proj1",
							ConfigRev:    "10001",
							ConfigDigest: "test config digest",
						},
						Realms: nil,
					},
				}
				err = updateAuthProjectRealms(ctx, updatedExpandedRealms, svcRev("perms:abc", "proj:def"), "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)

				actualChanges, err := generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "project realms changed", 2, actualChanges, []*AuthDBChange{{
					ChangeType:   ChangeProjectRealmsChanged,
					ConfigRevOld: "10001",
					ConfigRevNew: "10001",
				}})
			})

			t.Run("both project config and permissions config changed", func(t *ftt.Test) {
				updatedProj1Realms := &protocol.Realms{
					Permissions: makeTestPermissions("luci.dev.p2", "luci.dev.p1"),
					Realms: []*protocol.Realm{
						{
							Name: "proj1:@root",
							Bindings: []*protocol.Binding{
								{
									// Permissions p2, p1.
									Permissions: []uint32{0, 1},
									Principals:  []string{"group:gr1"},
								},
							},
						},
					},
				}
				updatedExpandedRealms := []*ExpandedRealms{
					{
						CfgRev: &RealmsCfgRev{
							ProjectID:    "proj1",
							ConfigRev:    "10002",
							ConfigDigest: "test config digest",
						},
						Realms: updatedProj1Realms,
					},
				}
				err = updateAuthProjectRealms(ctx, updatedExpandedRealms, svcRev("perms:new", "proj:new"), "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)

				actualChanges, err := generateChanges(ctx, 2)
				assert.Loosely(t, err, should.BeNil)
				validateChanges(ctx, "project realms changed and reevaluated", 2, actualChanges, []*AuthDBChange{
					{
						ChangeType:   ChangeProjectRealmsChanged,
						ConfigRevOld: "10001",
						ConfigRevNew: "10002",
					},
					{
						ChangeType:     ChangeProjectRealmsReevaluated,
						PermsRevOld:    "perms:abc",
						PermsRevNew:    "perms:new",
						ProjectsRevOld: "proj:def",
						ProjectsRevNew: "proj:new",
					},
				})
			})
		})

		t.Run("AuthRealmsGlobals changes", func(t *ftt.Test) {
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
			assert.Loosely(t, updateAuthRealmsGlobals(ctx, permCfg, "Go pRPC API"), should.BeNil)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
			actualChanges, err := generateChanges(ctx, 1)
			assert.Loosely(t, err, should.BeNil)
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
			assert.Loosely(t, updateAuthRealmsGlobals(ctx, permCfg, "Go pRPC API"), should.BeNil)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(4))
			actualChanges, err = generateChanges(ctx, 2)
			assert.Loosely(t, err, should.BeNil)
			validateChanges(ctx, "update realms globals, old config present", 2, actualChanges, []*AuthDBChange{{
				ChangeType:         ChangeRealmsGlobalsChanged,
				PermissionsChanged: []string{"test.perm.create"},
				PermissionsRemoved: []string{"test.perm.edit"},
			}})

			permCfg = &configspb.PermissionsConfig{
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
			assert.Loosely(t, updateAuthRealmsGlobals(ctx, permCfg, "Go pRPC API"), should.BeNil)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(6))
			actualChanges, err = generateChanges(ctx, 3)
			assert.Loosely(t, err, should.BeNil)
			validateChanges(ctx, "add perm to realms globals, old config present", 3, actualChanges, []*AuthDBChange{{
				ChangeType:         ChangeRealmsGlobalsChanged,
				PermissionsAdded:   []string{"test.perm.edit"},
				PermissionsChanged: []string{"test.perm.create"},
			}})
		})

		t.Run("Changelog generation cascades", func(t *ftt.Test) {
			// Create and add 3 AuthGroups.
			groupNames := []string{"group-1", "group-2", "group-3"}
			for _, groupName := range groupNames {
				ag := makeAuthGroup(ctx, groupName)
				_, err := CreateAuthGroup(ctx, ag, "Go pRPC API")
				assert.Loosely(t, err, should.BeNil)
			}
			// There should be a changelog task and replication task for
			// each group.
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(6))

			actualChanges, err := generateChanges(ctx, 1)
			assert.Loosely(t, err, should.BeNil)
			validateChanges(ctx, "created first group in cascade test", 1, actualChanges, []*AuthDBChange{{
				ChangeType: ChangeGroupCreated,
				Owners:     AdminGroup,
			}})
			// First change has no prior revision, so a task should not
			// have been added.
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(6))

			actualChanges, err = generateChanges(ctx, 3)
			assert.Loosely(t, err, should.BeNil)
			validateChanges(ctx, "created third group in cascade test", 3, actualChanges, []*AuthDBChange{{
				ChangeType: ChangeGroupCreated,
				Owners:     AdminGroup,
			}})
			// Changelog for rev 2 has not been generated yet, so there
			// should be an added task.
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(7))

			actualChanges, err = generateChanges(ctx, 2)
			assert.Loosely(t, err, should.BeNil)
			validateChanges(ctx, "created second group in cascade test", 2, actualChanges, []*AuthDBChange{{
				ChangeType: ChangeGroupCreated,
				Owners:     AdminGroup,
			}})
			// Changelog for rev 1 has already been generated, so a task
			// should not have been added.
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(7))
		})
	})
}

func TestAuthDBChangeProtoConversion(t *testing.T) {
	t.Parallel()

	ftt.Run("AuthDBChange ToProto works for non-membership changes", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"testers"},
		})

		testChange := testAuthDBIPAllowlistChange(ctx, t, 1001)
		actual, err := testChange.ToProto(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(&rpcpb.AuthDBChange{
			ChangeType:  "IPWL_CREATED",
			Comment:     "comment",
			Description: "description",
			Target:      "AuthIPWhitelist$a",
			AuthDbRev:   1001,
			Who:         "user:admin@example.com",
			When:        timestamppb.New(time.Date(2021, time.December, 12, 1, 0, 0, 0, time.UTC)),
			AppVersion:  "123-45abc",
		}))
	})

	ftt.Run("AuthDBChange ToProto applies privacy filter", t, func(t *ftt.Test) {
		t.Run("filters Google groups", func(t *ftt.Test) {
			ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{"testers"},
			})

			testChange := testAuthDBGroupChange(ctx, t, "AuthGroup$google/test@group.com", 1200, 1001)
			testChange.Members = []string{"joe.bloggs@group.com"}
			actual, err := testChange.ToProto(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&rpcpb.AuthDBChange{
				ChangeType:  "GROUP_MEMBERS_ADDED",
				Comment:     "comment",
				Target:      "AuthGroup$google/test@group.com",
				AuthDbRev:   1001,
				NumRedacted: 1,
				Who:         "user:test@example.com",
				When:        timestamppb.New(time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC)),
				AppVersion:  "123-45abc",
			}))
		})

		t.Run("Google groups can be sen by admin", func(t *ftt.Test) {
			ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
				Identity:       "user:admin@example.com",
				IdentityGroups: []string{AdminGroup},
			})

			testChange := testAuthDBGroupChange(ctx, t, "AuthGroup$google/test@group.com", 1200, 1001)
			testChange.Members = []string{"joe.bloggs@group.com"}
			actual, err := testChange.ToProto(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&rpcpb.AuthDBChange{
				ChangeType: "GROUP_MEMBERS_ADDED",
				Comment:    "comment",
				Target:     "AuthGroup$google/test@group.com",
				AuthDbRev:  1001,
				Members:    []string{"joe.bloggs@group.com"},
				Who:        "user:test@example.com",
				When:       timestamppb.New(time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC)),
				AppVersion: "123-45abc",
			}))
		})

		t.Run("shows TwoSync groups", func(t *ftt.Test) {
			ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{"testers"},
			})

			testChange := testAuthDBGroupChange(ctx, t, "AuthGroup$google/team@twosync.google.com", 1300, 1002)
			testChange.Members = []string{"user@google.com"}
			actual, err := testChange.ToProto(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&rpcpb.AuthDBChange{
				ChangeType: "GROUP_MEMBERS_REMOVED",
				Comment:    "comment",
				Target:     "AuthGroup$google/team@twosync.google.com",
				AuthDbRev:  1002,
				Members:    []string{"user@google.com"},
				Who:        "user:test@example.com",
				When:       timestamppb.New(time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC)),
				AppVersion: "123-45abc",
			}))
		})
	})
}

func TestPropertyMapHelpers(t *testing.T) {
	ftt.Run("getStringSliceProp robustly returns strings", t, func(t *ftt.Test) {
		pm := datastore.PropertyMap{}
		pm["testField"] = datastore.PropertySlice{
			datastore.MkPropertyNI([]byte("test data first entry")),
			datastore.MkPropertyNI("test data second entry"),
		}

		assert.Loosely(t, getStringSliceProp(pm, "testField"), should.Match([]string{
			"test data first entry",
			"test data second entry",
		}))
	})
}
