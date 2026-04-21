// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package realmsinternals

import (
	"slices"
	"strings"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/auth_service/testsupport"
)

func TestConditionsSet(t *testing.T) {
	t.Parallel()

	restriction := func(attr string, values []string) *realmsconf.Condition {
		return &realmsconf.Condition{
			Op: &realmsconf.Condition_Restrict{
				Restrict: &realmsconf.Condition_AttributeRestriction{
					Attribute: attr,
					Values:    values,
				},
			},
		}
	}

	t.Run("test key", func(t *testing.T) {
		cond1 := &protocol.Condition{
			Op: &protocol.Condition_Restrict{
				Restrict: &protocol.Condition_AttributeRestriction{
					Attribute: "attr",
					Values:    []string{"test"},
				},
			},
		}
		cond2 := &protocol.Condition{
			Op: &protocol.Condition_Restrict{
				Restrict: &protocol.Condition_AttributeRestriction{
					Attribute: "attr",
					Values:    []string{"test"},
				},
			},
		}
		// same contents == same key
		cond1Key, cond2Key := conditionKey(cond1), conditionKey(cond2)
		assert.Loosely(t, cond1Key, should.Equal(cond2Key))
		condEmpty := &protocol.Condition{}
		assert.Loosely(t, conditionKey(condEmpty), should.BeEmpty)
	})

	t.Run("errors", func(t *testing.T) {
		cs := &ConditionsSet{
			normalized: map[string]*conditionDetails{},
			conditions: map[*realmsconf.Condition]*conditionDetails{},
		}
		r1 := restriction("a", []string{"1", "2"})
		assert.Loosely(t, cs.addCond(r1), should.BeNil)
		assert.Loosely(t, cs.addCond(&realmsconf.Condition{}), should.NotBeNil)
	})

	t.Run("works", func(t *testing.T) {
		cs := &ConditionsSet{
			normalized: map[string]*conditionDetails{},
			conditions: map[*realmsconf.Condition]*conditionDetails{},
		}
		allAttrs := newAttrSet([]string{"a", "b"})
		r1 := restriction("b", []string{"1", "2"})
		r2 := restriction("a", []string{"2", "1", "1"})
		r3 := restriction("a", []string{"1", "2"})
		r4 := restriction("a", []string{"3", "4"})
		assert.Loosely(t, cs.addCond(r1), should.BeNil)
		assert.Loosely(t, cs.addCond(r1), should.BeNil)
		assert.Loosely(t, cs.addCond(r2), should.BeNil)
		assert.Loosely(t, cs.addCond(r3), should.BeNil)
		assert.Loosely(t, cs.addCond(r4), should.BeNil)
		out := cs.finalize()
		expected := []*protocol.Condition{
			{
				Op: &protocol.Condition_Restrict{
					Restrict: &protocol.Condition_AttributeRestriction{
						Attribute: "a",
						Values:    []string{"1", "2"},
					},
				},
			},
			{
				Op: &protocol.Condition_Restrict{
					Restrict: &protocol.Condition_AttributeRestriction{
						Attribute: "a",
						Values:    []string{"3", "4"},
					},
				},
			},
			{
				Op: &protocol.Condition_Restrict{
					Restrict: &protocol.Condition_AttributeRestriction{
						Attribute: "b",
						Values:    []string{"1", "2"},
					},
				},
			},
		}
		assert.Loosely(t, out, should.Match(expected))
		assert.Loosely(t, cs.indexes([]*realmsconf.Condition{r1}, &allAttrs), should.Match([]uint32{2}))
		assert.Loosely(t, cs.indexes([]*realmsconf.Condition{r2}, &allAttrs), should.Match([]uint32{0}))
		assert.Loosely(t, cs.indexes([]*realmsconf.Condition{r3}, &allAttrs), should.Match([]uint32{0}))
		assert.Loosely(t, cs.indexes([]*realmsconf.Condition{r4}, &allAttrs), should.Match([]uint32{1}))
		inds := cs.indexes([]*realmsconf.Condition{r1, r2, r3, r4}, &allAttrs)
		assert.Loosely(t, inds, should.Match([]uint32{0, 1, 2}))
	})
}

func TestRolesExpander(t *testing.T) {
	t.Parallel()

	permDB := testsupport.PermissionsDB(false)

	rolePerms := func(t *testing.T, r *RolesExpander, role string) []string {
		t.Helper()
		perms := stringset.New(0)
		assert.NoErr(t, r.rolePermissions(role, perms))
		return perms.ToSortedSlice()
	}

	t.Run("errors", func(t *testing.T) {
		permDB := testsupport.PermissionsDB(false)
		r := &RolesExpander{
			permissionsDB: permDB.Permissions,
			builtinRoles:  permDB.Roles,
			customRoles:   map[string]*realmsconf.CustomRole{},
			permissions:   map[string]permissionDetails{},
			roles:         map[string]roleDetails{},
		}
		_, err := r.roleDetails("role/notbuiltin")
		assert.Loosely(t, err, should.ErrLike(ErrRoleNotFound))
		_, err = r.roleDetails("customRole/notarole")
		assert.Loosely(t, err, should.ErrLike(ErrRoleNotFound))
		_, err = r.roleDetails("notarole/test")
		assert.Loosely(t, err, should.ErrLike(ErrImpossibleRole))
	})

	t.Run("works", func(t *testing.T) {
		r := &RolesExpander{
			permissionsDB: permDB.Permissions,
			builtinRoles:  permDB.Roles,
			permissions:   map[string]permissionDetails{},
			roles:         map[string]roleDetails{},
			customRoles: map[string]*realmsconf.CustomRole{
				"customRole/custom1": {
					Name:        "customRole/custom1",
					Extends:     []string{"role/dev.a", "customRole/custom2", "customRole/custom3"},
					Permissions: []string{"luci.dev.p1", "luci.dev.p4"},
				},
				"customRole/custom2": {
					Name:        "customRole/custom2",
					Extends:     []string{"customRole/custom3"},
					Permissions: []string{"luci.dev.p4"},
				},
				"customRole/custom3": {
					Name:        "customRole/custom3",
					Extends:     []string{"role/dev.b"},
					Permissions: []string{"luci.dev.p5"},
				},
			},
		}

		expected := []struct {
			role    string
			perms   []string
			indexes []uint32
			groups  map[string][]uint32
		}{
			{
				role:    "role/dev.a",
				perms:   []string{"luci.dev.p1", "luci.dev.p2"},
				indexes: []uint32{0, 1},
				groups:  map[string][]uint32{"a1,a2": {1}, "a1,root": {0}},
			},
			{
				role:    "role/dev.b",
				perms:   []string{"luci.dev.p2", "luci.dev.p3"},
				indexes: []uint32{1, 2},
				groups:  map[string][]uint32{"": {2}, "a1,a2": {1}},
			},
			{
				role: "customRole/custom1",
				perms: []string{
					"luci.dev.p1",
					"luci.dev.p2",
					"luci.dev.p3",
					"luci.dev.p4",
					"luci.dev.p5",
				},
				indexes: []uint32{0, 1, 2, 3, 4},
				groups:  map[string][]uint32{"": {2, 3, 4}, "a1,a2": {1}, "a1,root": {0}},
			},
			{
				role: "customRole/custom2",
				perms: []string{
					"luci.dev.p2",
					"luci.dev.p3",
					"luci.dev.p4",
					"luci.dev.p5",
				},
				indexes: []uint32{1, 2, 3, 4},
				groups:  map[string][]uint32{"": {2, 3, 4}, "a1,a2": {1}},
			},
			{
				role: "customRole/custom3",
				perms: []string{
					"luci.dev.p2",
					"luci.dev.p3",
					"luci.dev.p5",
				},
				indexes: []uint32{1, 2, 4},
				groups:  map[string][]uint32{"": {2, 4}, "a1,a2": {1}},
			},
		}

		// Note this also prefill all permission indexes.
		for _, tc := range expected {
			t.Run(tc.role, func(t *testing.T) {
				assert.That(t, rolePerms(t, r, tc.role), should.Match(tc.perms))
				_, err := r.roleDetails(tc.role)
				assert.NoErr(t, err)
			})
		}

		// Get the final set of all permission used across all roles.
		perms, mapping := r.sortedPermissions()
		assert.That(t, perms, should.Match([]*protocol.Permission{
			{Name: "luci.dev.p1", Attributes: []string{"a1", "root"}},
			{Name: "luci.dev.p2", Attributes: []string{"a1", "a2"}},
			{Name: "luci.dev.p3"},
			{Name: "luci.dev.p4"},
			{Name: "luci.dev.p5"},
		}))

		reMap := func(role roleDetails) (all []uint32, groups map[string][]uint32) {
			groups = map[string][]uint32{}
			for _, gr := range role.groups {
				var group []uint32
				for old := range gr.perms.set {
					group = append(group, mapping[old])
					all = append(all, mapping[old])
				}
				slices.Sort(group)
				groups[strings.Join(gr.attrs.asSet, ",")] = group
			}
			slices.Sort(all)
			return all, groups
		}

		// Now that we have stable final indexes for permissions, verify
		// roleDetails returns expected index sets.
		for _, tc := range expected {
			t.Run(tc.role+"_indexes", func(t *testing.T) {
				role, err := r.roleDetails(tc.role)
				assert.NoErr(t, err)
				all, groups := reMap(role)
				assert.That(t, all, should.Match(tc.indexes))
				assert.That(t, groups, should.Match(tc.groups))
			})
		}
	})
}

func TestRealmsExpander(t *testing.T) {
	t.Parallel()

	t.Run("test perPrincipalBindings", func(t *testing.T) {
		t.Run("errors", func(t *testing.T) {
			t.Run("realm not found", func(t *testing.T) {
				r := &RealmsExpander{}
				_, err := r.perPrincipalBindings("test", nil)
				assert.Loosely(t, err, should.ErrLike("realm test not found in RealmsExpander"))
			})

			t.Run("parent not found", func(t *testing.T) {
				r := &RealmsExpander{
					realms: map[string]*realmsconf.Realm{
						"test": {
							Name:    "test",
							Extends: []string{"test-2"},
						},
					},
				}

				_, err := r.perPrincipalBindings("test", nil)
				assert.Loosely(t, err, should.ErrLike(`parent realm "test": realm @root not found in RealmsExpander`))
			})

			t.Run("realm name mismatch", func(t *testing.T) {
				r := &RealmsExpander{
					realms: map[string]*realmsconf.Realm{
						"test": {
							Name: "not-test",
						},
					},
				}
				_, err := r.perPrincipalBindings("test", nil)
				assert.Loosely(t, err, should.ErrLike("given realm: test does not match name found internally: not-test"))
			})

			t.Run("permissions fetch issue (ErrRoleNotfound)", func(t *testing.T) {
				r := &RealmsExpander{
					rolesExpander: &RolesExpander{
						builtinRoles: map[string]*permissions.Role{},
						permissions:  map[string]permissionDetails{},
						roles:        map[string]roleDetails{},
					},
					realms: map[string]*realmsconf.Realm{
						"@root": {
							Name: "@root",
						},
						"test": {
							Name:    "test",
							Extends: []string{},
							Bindings: []*realmsconf.Binding{
								{
									Role:       "role/test-role",
									Principals: []string{"test-project"},
								},
							},
							EnforceInService: []string{},
						},
					},
				}
				_, err := r.perPrincipalBindings("test", nil)
				assert.Loosely(t, err, should.ErrLike("role does not exist in internal representation: builtinRole: role/test-role"))
			})
		})
	})
}
