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
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/internal/permissions"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTakeSnapshot(t *testing.T) {
	t.Parallel()

	const testAuthDBRev = 12345

	Convey("Testing TakeSnapshot", t, func() {
		ctx := memory.Use(context.Background())

		_, err := TakeSnapshot(ctx)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		realmsGlobals := testAuthRealmsGlobals(ctx)
		perms := makeTestPermissions("luci.dev.p1", "luci.dev.p2")
		realmsGlobals.PermissionsList = &permissions.PermissionsList{
			Permissions: perms,
		}
		projectRealms1 := testAuthProjectRealms(ctx, "project-1")
		projectRealms1.Realms, err = proto.Marshal(&protocol.Realms{
			Permissions: makeTestPermissions("luci.dev.p2"),
			Conditions:  makeTestConditions("a", "c"),
			Realms: []*protocol.Realm{
				{
					Name: "project-1:@root",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{0},
							Conditions:  []uint32{0, 1},
							Principals:  []string{"group:group-1"},
						},
					},
				},
			},
		})
		So(err, ShouldBeNil)
		projectRealms2 := testAuthProjectRealms(ctx, "project-2")
		projectRealms2.Realms, err = proto.Marshal(&protocol.Realms{
			Permissions: makeTestPermissions("luci.dev.p1"),
			Conditions:  makeTestConditions("b"),
			Realms: []*protocol.Realm{
				{
					Name: "project-2:@root",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{0},
							Conditions:  []uint32{0},
							Principals:  []string{"group:group-2"},
						},
					},
				},
			},
		})
		So(err, ShouldBeNil)
		So(datastore.Put(ctx,
			testAuthReplicationState(ctx, testAuthDBRev),
			testAuthGlobalConfig(ctx),
			testAuthGroup(ctx, "group-2"),
			testAuthGroup(ctx, "group-1"),
			testIPAllowlist(ctx, "ip-allowlist-2", nil),
			testIPAllowlist(ctx, "ip-allowlist-1", nil),
			realmsGlobals,
			projectRealms2,
			projectRealms1,
		), ShouldBeNil)

		snap, err := TakeSnapshot(ctx)
		So(err, ShouldBeNil)

		So(snap, ShouldResembleProto, &Snapshot{
			ReplicationState: testAuthReplicationState(ctx, 12345),
			GlobalConfig:     testAuthGlobalConfig(ctx),
			Groups: []*AuthGroup{
				testAuthGroup(ctx, "group-1"),
				testAuthGroup(ctx, "group-2"),
			},
			IPAllowlists: []*AuthIPAllowlist{
				testIPAllowlist(ctx, "ip-allowlist-1", nil),
				testIPAllowlist(ctx, "ip-allowlist-2", nil),
			},
			RealmsGlobals: realmsGlobals,
			ProjectRealms: []*AuthProjectRealms{
				projectRealms1,
				projectRealms2,
			},
		})

		Convey("ToAuthDBProto", func() {
			groupProto := func(name string) *protocol.AuthGroup {
				return &protocol.AuthGroup{
					Name: name,
					Members: []string{
						fmt.Sprintf("user:%s-m1@example.com", name),
						fmt.Sprintf("user:%s-m2@example.com", name),
					},
					Globs:       []string{"user:*@example.com"},
					Nested:      []string{"nested-" + name},
					Description: fmt.Sprintf("This is a test auth group %q.", name),
					CreatedTs:   testCreatedTS.UnixNano() / 1000,
					CreatedBy:   "user:test-creator@example.com",
					ModifiedTs:  testModifiedTS.UnixNano() / 1000,
					ModifiedBy:  "user:test-modifier@example.com",
					Owners:      "owners-" + name,
				}
			}

			allowlistProto := func(name string) *protocol.AuthIPWhitelist {
				return &protocol.AuthIPWhitelist{
					Name: name,
					Subnets: []string{
						"127.0.0.1/10",
						"127.0.0.1/20",
					},
					Description: fmt.Sprintf("This is a test AuthIPAllowlist %q.", name),
					CreatedTs:   testCreatedTS.UnixNano() / 1000,
					CreatedBy:   "user:test-creator@example.com",
					ModifiedTs:  testModifiedTS.UnixNano() / 1000,
					ModifiedBy:  "user:test-modifier@example.com",
				}
			}

			expectedMergedRealmsProto := &protocol.Realms{
				ApiVersion:  RealmsAPIVersion,
				Permissions: makeTestPermissions("luci.dev.p1", "luci.dev.p2"),
				Conditions:  makeTestConditions("a", "c", "b"),
				Realms: []*protocol.Realm{
					{
						Name: "project-1:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{1},
								Conditions:  []uint32{0, 1},
								Principals:  []string{"group:group-1"},
							},
						},
					},
					{
						Name: "project-2:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Conditions:  []uint32{2},
								Principals:  []string{"group:group-2"},
							},
						},
					},
				},
			}

			authDBProto, err := snap.ToAuthDBProto(ctx, false)
			So(err, ShouldBeNil)
			So(authDBProto, ShouldResembleProto, &protocol.AuthDB{
				OauthClientId:     "test-client-id",
				OauthClientSecret: "test-client-secret",
				OauthAdditionalClientIds: []string{
					"additional-client-id-0",
					"additional-client-id-1",
				},
				TokenServerUrl: "https://token-server.example.com",
				SecurityConfig: testSecurityConfigBlob(),
				Groups: []*protocol.AuthGroup{
					groupProto("group-1"),
					groupProto("group-2"),
				},
				IpWhitelists: []*protocol.AuthIPWhitelist{
					allowlistProto("ip-allowlist-1"),
					allowlistProto("ip-allowlist-2"),
				},
				Realms: expectedMergedRealmsProto,
			})

			Convey("empty string fields are set to `empty`", func() {
				sparseGlobalConfig := &AuthGlobalConfig{
					Kind:                     "AuthGlobalConfig",
					ID:                       "root",
					AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
				}
				sparseAuthGroup1 := testAuthGroup(ctx, "group-1")
				sparseAuthGroup1.Description = ""
				sparseSnapshot := &Snapshot{
					ReplicationState: testAuthReplicationState(ctx, 12345),
					GlobalConfig:     sparseGlobalConfig,
					Groups: []*AuthGroup{
						sparseAuthGroup1,
						testAuthGroup(ctx, "group-2"),
					},
				}

				expectedGroup1 := groupProto("group-1")
				expectedGroup1.Description = "empty"
				authDBProto, err := sparseSnapshot.ToAuthDBProto(ctx, false)
				So(err, ShouldBeNil)
				So(authDBProto, ShouldResembleProto, &protocol.AuthDB{
					OauthClientId:     "empty",
					OauthClientSecret: "empty",
					TokenServerUrl:    "empty",
					Groups: []*protocol.AuthGroup{
						expectedGroup1,
						groupProto("group-2"),
					},
					Realms: &protocol.Realms{
						ApiVersion: RealmsAPIVersion,
					},
				})
			})
		})

		Convey("ToAuthDB", func() {
			db, err := snap.ToAuthDB(ctx, false)
			So(err, ShouldBeNil)
			So(db.Rev, ShouldEqual, testAuthDBRev)
		})
	})
}
