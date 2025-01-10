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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/internal/permissions"
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

	ftt.Run("Merging realms", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("empty permissions and project realms", func(t *ftt.Test) {
			realmsGlobals := &AuthRealmsGlobals{
				PermissionsList: &permissions.PermissionsList{
					Permissions: makeTestPermissions(),
				},
			}
			projectRealms := []*AuthProjectRealms{}
			merged, err := MergeRealms(ctx, realmsGlobals, projectRealms)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, merged, should.Match(&protocol.Realms{
				ApiVersion:  RealmsAPIVersion,
				Permissions: []*protocol.Permission{},
				Realms:      []*protocol.Realm{},
			}))
		})

		t.Run("permissions are remapped", func(t *ftt.Test) {
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
			blob, err := ToStorableRealms(proj1Realms)
			assert.Loosely(t, err, should.BeNil)
			projectRealms := []*AuthProjectRealms{
				{ID: "proj1", Realms: blob},
			}

			expectedRealms := &protocol.Realms{
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
			}

			t.Run("when using PermissionsList", func(t *ftt.Test) {
				realmsGlobals := &AuthRealmsGlobals{
					PermissionsList: &permissions.PermissionsList{
						Permissions: makeTestPermissions("luci.dev.p1", "luci.dev.p2"),
					},
				}

				merged, err := MergeRealms(ctx, realmsGlobals, projectRealms)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, merged, should.Match(expectedRealms))
			})
		})

		t.Run("conditions are remapped", func(t *ftt.Test) {
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
			blob1, err := ToStorableRealms(proj1Realms)
			assert.Loosely(t, err, should.BeNil)
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
			blob2, err := ToStorableRealms(proj2Realms)
			assert.Loosely(t, err, should.BeNil)
			projectRealms := []*AuthProjectRealms{
				{ID: "proj1", Realms: blob1},
				{ID: "proj2", Realms: blob2},
			}

			merged, err := MergeRealms(ctx, realmsGlobals, projectRealms)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, merged, should.Match(&protocol.Realms{
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
			}))
		})

		t.Run("permissions across multiple projects are remapped", func(t *ftt.Test) {
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
			blob1, err := ToStorableRealms(proj1Realms)
			assert.Loosely(t, err, should.BeNil)
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
			blob2, err := ToStorableRealms(proj2Realms)
			assert.Loosely(t, err, should.BeNil)
			projectRealms := []*AuthProjectRealms{
				{ID: "proj1", Realms: blob1},
				{ID: "proj2", Realms: blob2},
			}

			merged, err := MergeRealms(ctx, realmsGlobals, projectRealms)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, merged, should.Match(&protocol.Realms{
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
			}))
		})

		t.Run("Realm name should have matching project ID prefix", func(t *ftt.Test) {
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
			blob, err := ToStorableRealms(proj1Realms)
			assert.Loosely(t, err, should.BeNil)
			projectRealms := []*AuthProjectRealms{
				{ID: "proj1", Realms: blob},
			}

			merged, err := MergeRealms(ctx, realmsGlobals, projectRealms)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, merged, should.BeNil)
		})
	})
}
