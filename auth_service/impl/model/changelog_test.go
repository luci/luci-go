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
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
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
