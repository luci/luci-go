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
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/internal/permissions"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// makeTestPermissions creates permissions with the given permission
// names for testing.
func makeTestPermissions(names ...string) []*protocol.Permission {
	perms := make([]*protocol.Permission, len(names))
	for i, name := range names {
		perms[i] = &protocol.Permission{Name: name}
	}
	return perms
}

// makeTestConditions creates conditions with the given attribute names
// for testing.
func makeTestConditions(names ...string) []*protocol.Condition {
	conds := make([]*protocol.Condition, len(names))
	for i, name := range names {
		conds[i] = &protocol.Condition{
			Op: &protocol.Condition_Restrict{
				Restrict: &protocol.Condition_AttributeRestriction{
					Attribute: name,
					Values:    []string{"x", "y", "z"},
				},
			},
		}
	}
	return conds
}

func TestMergeRealms(t *testing.T) {
	t.Parallel()

	Convey("Merging realms", t, func() {
		Convey("empty permissions and project realms", func() {
			realmsGlobals := &AuthRealmsGlobals{
				PermissionsList: &permissions.PermissionsList{
					Permissions: makeTestPermissions(),
				},
			}
			projectRealms := []*AuthProjectRealms{}
			merged, err := MergeRealms(realmsGlobals, projectRealms)
			So(err, ShouldBeNil)
			So(merged, ShouldResembleProto, &protocol.Realms{
				ApiVersion:  RealmsAPIVersion,
				Permissions: []*protocol.Permission{},
				Realms:      []*protocol.Realm{},
			})
		})

		Convey("permissions are remapped", func() {
			realmsGlobals := &AuthRealmsGlobals{
				PermissionsList: &permissions.PermissionsList{
					Permissions: makeTestPermissions("luci.dev.p1", "luci.dev.p2"),
				},
			}
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
							{
								// Permission z only; should be dropped.
								Permissions: []uint32{1},
								Principals:  []string{"group:gr2"},
							},
							{
								// Permission p1.
								Permissions: []uint32{2},
								Principals:  []string{"group:gr3"},
							},
						},
					},
				},
			}
			marshalled, err := proto.Marshal(proj1Realms)
			So(err, ShouldBeNil)
			projectRealms := []*AuthProjectRealms{
				{ID: "proj1", Realms: marshalled},
			}

			merged, err := MergeRealms(realmsGlobals, projectRealms)
			So(err, ShouldBeNil)
			So(merged, ShouldResembleProto, &protocol.Realms{
				ApiVersion:  RealmsAPIVersion,
				Permissions: makeTestPermissions("luci.dev.p1", "luci.dev.p2"),
				Realms: []*protocol.Realm{
					{
						Name: "proj1:@root",
						Bindings: []*protocol.Binding{
							{
								// Permission p1.
								Permissions: []uint32{0},
								Principals:  []string{"group:gr3"},
							},
							{
								// Permissions p1, p2.
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
						},
					},
				},
			})
		})

		Convey("conditions are remapped", func() {
			realmsGlobals := &AuthRealmsGlobals{
				PermissionsList: &permissions.PermissionsList{
					Permissions: makeTestPermissions("luci.dev.p1"),
				},
			}

			// Set up project realms.
			proj1Realms := &protocol.Realms{
				Permissions: makeTestPermissions("luci.dev.p1"),
				Conditions:  makeTestConditions("a", "b"),
				Realms: []*protocol.Realm{
					{
						Name: "proj1:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Conditions:  []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
						},
					},
				},
			}
			marshalledProj1, err := proto.Marshal(proj1Realms)
			So(err, ShouldBeNil)
			proj2Realms := &protocol.Realms{
				Permissions: makeTestPermissions("luci.dev.p1"),
				Conditions:  makeTestConditions("c", "a", "b"),
				Realms: []*protocol.Realm{
					{
						Name: "proj2:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								// Condition c.
								Conditions: []uint32{0},
								Principals: []string{"group:gr2"},
							},
							{
								Permissions: []uint32{0},
								// Condition a.
								Conditions: []uint32{1},
								Principals: []string{"group:gr3"},
							},
							{
								Permissions: []uint32{0},
								// Conditions c, a, b.
								Conditions: []uint32{0, 1, 2},
								Principals: []string{"group:gr4"},
							},
						},
					},
				},
			}
			marshalledProj2, err := proto.Marshal(proj2Realms)
			So(err, ShouldBeNil)
			projectRealms := []*AuthProjectRealms{
				{ID: "proj1", Realms: marshalledProj1},
				{ID: "proj2", Realms: marshalledProj2},
			}

			merged, err := MergeRealms(realmsGlobals, projectRealms)
			So(err, ShouldBeNil)
			So(merged, ShouldResembleProto, &protocol.Realms{
				ApiVersion:  RealmsAPIVersion,
				Permissions: makeTestPermissions("luci.dev.p1"),
				Conditions:  makeTestConditions("a", "b", "c"),
				Realms: []*protocol.Realm{
					{
						Name: "proj1:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Conditions:  []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
						},
					},
					{
						Name: "proj2:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								// Condition a.
								Conditions: []uint32{0},
								Principals: []string{"group:gr3"},
							},
							{
								Permissions: []uint32{0},
								// Conditions a, b, c.
								Conditions: []uint32{0, 1, 2},
								Principals: []string{"group:gr4"},
							},
							{
								Permissions: []uint32{0},
								// Condition c.
								Conditions: []uint32{2},
								Principals: []string{"group:gr2"},
							},
						},
					},
				},
			})
		})

		Convey("permissions across multiple projects are remapped", func() {
			realmsGlobals := &AuthRealmsGlobals{
				PermissionsList: &permissions.PermissionsList{
					Permissions: makeTestPermissions("luci.dev.p1", "luci.dev.p2", "luci.dev.p3"),
				},
			}

			// Set up project realms.
			proj1Realms := &protocol.Realms{
				Permissions: makeTestPermissions("luci.dev.p1", "luci.dev.p2"),
				Realms: []*protocol.Realm{
					{
						Name: "proj1:@root",
						Bindings: []*protocol.Binding{
							{
								// Permissions p1 and p2.
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
						},
						Data: &protocol.RealmData{
							EnforceInService: []string{"a"},
						},
					},
				},
			}
			marshalledProj1, err := proto.Marshal(proj1Realms)
			So(err, ShouldBeNil)
			proj2Realms := &protocol.Realms{
				Permissions: makeTestPermissions("luci.dev.p2", "luci.dev.p3"),
				Realms: []*protocol.Realm{
					{
						Name: "proj2:@root",
						Bindings: []*protocol.Binding{
							{
								// Permissions p2 and p3.
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr2"},
							},
						},
					},
				},
			}
			marshalledProj2, err := proto.Marshal(proj2Realms)
			So(err, ShouldBeNil)
			projectRealms := []*AuthProjectRealms{
				{ID: "proj1", Realms: marshalledProj1},
				{ID: "proj2", Realms: marshalledProj2},
			}

			merged, err := MergeRealms(realmsGlobals, projectRealms)
			So(err, ShouldBeNil)
			So(merged, ShouldResembleProto, &protocol.Realms{
				ApiVersion:  RealmsAPIVersion,
				Permissions: makeTestPermissions("luci.dev.p1", "luci.dev.p2", "luci.dev.p3"),
				Realms: []*protocol.Realm{
					{
						Name: "proj1:@root",
						Bindings: []*protocol.Binding{
							{
								// Permissions p1 and p2.
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
						},
						Data: &protocol.RealmData{
							EnforceInService: []string{"a"},
						},
					},
					{
						Name: "proj2:@root",
						Bindings: []*protocol.Binding{
							{
								// Permissions p2 and p3.
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr2"},
							},
						},
					},
				},
			})
		})

		Convey("Realm name should have matching project ID prefix", func() {
			realmsGlobals := &AuthRealmsGlobals{
				PermissionsList: &permissions.PermissionsList{
					Permissions: makeTestPermissions("luci.dev.p1"),
				},
			}
			proj1Realms := &protocol.Realms{
				Permissions: makeTestPermissions("luci.dev.p1"),
				Realms: []*protocol.Realm{
					{
						Name: "proj2:@root",
					},
				},
			}
			marshalled, err := proto.Marshal(proj1Realms)
			So(err, ShouldBeNil)
			projectRealms := []*AuthProjectRealms{
				{ID: "proj1", Realms: marshalled},
			}

			merged, err := MergeRealms(realmsGlobals, projectRealms)
			So(err, ShouldNotBeNil)
			So(merged, ShouldBeNil)
		})
	})
}
