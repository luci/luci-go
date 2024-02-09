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

package impl

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/internal/permissions"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	testPerm1 = realms.RegisterPermission("testing.tests.perm1")
	testPerm2 = realms.RegisterPermission("testing.tests.perm2")
)

func makeTestPermissions(names ...string) []*protocol.Permission {
	perms := make([]*protocol.Permission, len(names))
	for i, name := range names {
		perms[i] = &protocol.Permission{Name: name}
	}
	return perms
}

func TestAuthDBProvider(t *testing.T) {
	t.Setenv(model.DryRunCronRealmsEnvVar, "false")

	Convey("AuthDBProvider works", t, func() {
		ctx := memory.Use(context.Background())
		authDB := &AuthDBProvider{}

		putRev := func(rev int64, members []string) {
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				globals := &model.AuthGlobalConfig{}
				state := &model.AuthReplicationState{
					AuthDBRev: rev,
					Parent:    model.RootKey(ctx),
				}
				group := &model.AuthGroup{
					ID:      "test-group",
					Parent:  model.RootKey(ctx),
					Members: members,
				}
				realmsGlobals := &model.AuthRealmsGlobals{
					Kind:   "AuthRealmsGlobals",
					ID:     "globals",
					Parent: model.RootKey(ctx),
					PermissionsList: &permissions.PermissionsList{
						Permissions: makeTestPermissions(testPerm1.String(), testPerm2.String()),
					},
				}
				// Grant all members of test-group testPerm2 permission
				// in project "test-project" root realm.
				marshalled, marshalErr := proto.Marshal(&protocol.Realms{
					Permissions: makeTestPermissions(testPerm2.String()),
					Realms: []*protocol.Realm{
						{
							Name: "test-project:@root",
							Bindings: []*protocol.Binding{
								{
									Permissions: []uint32{0},
									Principals:  []string{"group:test-group"},
								},
							},
						},
					},
				})
				So(marshalErr, ShouldBeNil)
				projectRealms := &model.AuthProjectRealms{
					Kind:   "AuthProjectRealms",
					ID:     "test-project",
					Parent: model.RootKey(ctx),
					Realms: marshalled,
				}
				return datastore.Put(ctx, globals, state, group, realmsGlobals, projectRealms)
			}, nil)
			So(err, ShouldBeNil)
		}

		// Initial revision.
		putRev(1000, []string{"user:a@example.com"})

		// Got it.
		db1, err := authDB.GetAuthDB(ctx)
		So(err, ShouldBeNil)
		So(authdb.Revision(db1), ShouldEqual, 1000)

		// Works.
		yes, err := db1.IsMember(ctx, "user:a@example.com", []string{"test-group"})
		So(err, ShouldBeNil)
		So(yes, ShouldBeTrue)

		// Check permission which hasn't been granted.
		allowed, err := db1.HasPermission(ctx, "user:a@example.com", testPerm1, "test-project:@root", nil)
		So(err, ShouldBeNil)
		So(allowed, ShouldBeFalse)
		// Check permission which has been granted.
		allowed, err = db1.HasPermission(ctx, "user:a@example.com", testPerm2, "test-project:@root", nil)
		So(err, ShouldBeNil)
		So(allowed, ShouldBeTrue)
		// Check realms fall back to @root.
		allowed, err = db1.HasPermission(ctx, "user:a@example.com", testPerm2, "test-project:unknown", nil)
		So(err, ShouldBeNil)
		So(allowed, ShouldBeTrue)

		// Calling again returns the exact same object.
		db2, err := authDB.GetAuthDB(ctx)
		So(err, ShouldBeNil)
		So(db2, ShouldEqual, db1)

		// Updated.
		putRev(1001, nil)

		// Got the new one.
		db3, err := authDB.GetAuthDB(ctx)
		So(err, ShouldBeNil)
		So(authdb.Revision(db3), ShouldEqual, 1001)

		// The group there is updated too.
		yes, err = db3.IsMember(ctx, "user:a@example.com", []string{"test-group"})
		So(err, ShouldBeNil)
		So(yes, ShouldBeFalse)

		// Check permission which hasn't been granted, as the member is
		// no longer in the group.
		allowed, err = db3.HasPermission(ctx, "user:a@example.com", testPerm2, "test-project:@root", nil)
		So(err, ShouldBeNil)
		So(allowed, ShouldBeFalse)
	})
}
