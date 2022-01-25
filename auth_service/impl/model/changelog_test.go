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

// TODO(xinyuoffline): Add more change types for testing.

func testAuthDBChange(ctx context.Context, target string, changeType ChangeType, authDBRev int64) *AuthDBChange {
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

////////////////////////////////////////////////////////////////////////////////

func TestGetAllAuthDBChange(t *testing.T) {
	t.Parallel()
	Convey("Testing GetAllAuthDBChange", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)

		So(datastore.Put(ctx,
			testAuthDBChange(ctx, "AuthGroup$groupA", ChangeGroupCreated, 1120),
			testAuthDBChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1120),
			testAuthDBChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
			testAuthDBChange(ctx, "AuthGroup$groupC", ChangeGroupMembersAdded, 1135),
			testAuthDBChange(ctx, "AuthGroup$groupC", ChangeGroupOwnersChanged, 1110),
		), ShouldBeNil)

		Convey("Sort by key", func() {
			changes, err := GetAllAuthDBChange(ctx, "", 0)
			So(err, ShouldBeNil)
			So(changes, ShouldResemble, []*AuthDBChange{
				testAuthDBChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
				testAuthDBChange(ctx, "AuthGroup$groupC", ChangeGroupMembersAdded, 1135),
				testAuthDBChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1120),
				testAuthDBChange(ctx, "AuthGroup$groupA", ChangeGroupCreated, 1120),
				testAuthDBChange(ctx, "AuthGroup$groupC", ChangeGroupOwnersChanged, 1110),
			})
		})
		Convey("Filter by target", func() {
			changes, err := GetAllAuthDBChange(ctx, "AuthGroup$groupB", 0)
			So(err, ShouldBeNil)
			So(changes, ShouldResemble, []*AuthDBChange{
				testAuthDBChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
				testAuthDBChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1120),
			})
		})
		Convey("Filter by authDBRev", func() {
			changes, err := GetAllAuthDBChange(ctx, "", 1136)
			So(err, ShouldBeNil)
			So(changes, ShouldResemble, []*AuthDBChange{
				testAuthDBChange(ctx, "AuthGroup$groupB", ChangeGroupMembersAdded, 1136),
			})
		})
		Convey("Return error when target is invalid", func() {
			_, err := GetAllAuthDBChange(ctx, "groupname", 0)
			So(err, ShouldErrLike, "Invalid target groupname")
		})
	})
}
