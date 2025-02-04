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
	"testing"

	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/common/testing/ftt"
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
	ftt.Run("test key", t, func(t *ftt.Test) {
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
	ftt.Run("errors", t, func(t *ftt.Test) {
		cs := &ConditionsSet{
			normalized:   map[string]*conditionMapTuple{},
			indexMapping: map[*realmsconf.Condition]uint32{},
		}
		r1 := restriction("a", []string{"1", "2"})
		r2 := restriction("b", []string{"1"})
		assert.Loosely(t, cs.addCond(r1), should.BeNil)
		cs.finalize()
		assert.Loosely(t, cs.addCond(r2), should.Equal(ErrFinalized))
	})
	ftt.Run("works", t, func(t *ftt.Test) {
		cs := &ConditionsSet{
			normalized:   map[string]*conditionMapTuple{},
			indexMapping: map[*realmsconf.Condition]uint32{},
			finalized:    false,
		}
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
		assert.Loosely(t, cs.indexes([]*realmsconf.Condition{r1}), should.Match([]uint32{2}))
		assert.Loosely(t, cs.indexes([]*realmsconf.Condition{r2}), should.Match([]uint32{0}))
		assert.Loosely(t, cs.indexes([]*realmsconf.Condition{r3}), should.Match([]uint32{0}))
		assert.Loosely(t, cs.indexes([]*realmsconf.Condition{r4}), should.Match([]uint32{1}))
		inds := cs.indexes([]*realmsconf.Condition{r1, r2, r3, r4})
		assert.Loosely(t, inds, should.Match([]uint32{0, 1, 2}))
	})
}
func TestRolesExpander(t *testing.T) {
	t.Parallel()
	ftt.Run("errors", t, func(t *ftt.Test) {
		permDB := testsupport.PermissionsDB(false)
		r := &RolesExpander{
			builtinRoles: permDB.Roles,
			customRoles:  map[string]*realmsconf.CustomRole{},
			permissions:  map[string]uint32{},
			roles:        map[string]*indexSet{},
		}
		_, err := r.role("role/notbuiltin")
		assert.Loosely(t, err, should.ErrLike(ErrRoleNotFound))
		_, err = r.role("customRole/notarole")
		assert.Loosely(t, err, should.ErrLike(ErrRoleNotFound))
		_, err = r.role("notarole/test")
		assert.Loosely(t, err, should.ErrLike(ErrImpossibleRole))
	})
	ftt.Run("test builtin roles works", t, func(t *ftt.Test) {
		permDB := testsupport.PermissionsDB(false)
		r := &RolesExpander{
			builtinRoles: permDB.Roles,
			permissions:  map[string]uint32{},
			roles:        map[string]*indexSet{},
		}
		actual, err := r.role("role/dev.a")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(IndexSetFromSlice([]uint32{0, 1})))
		actual, err = r.role("role/dev.b")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(IndexSetFromSlice([]uint32{1, 2})))
		perms, mapping := r.sortedPermissions()
		assert.Loosely(t, perms, should.Match([]string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"}))
		assert.Loosely(t, mapping, should.Match([]uint32{0, 1, 2}))
	})
	ftt.Run("test custom roles works", t, func(t *ftt.Test) {
		permDB := testsupport.PermissionsDB(false)
		r := &RolesExpander{
			builtinRoles: permDB.Roles,
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
			permissions: map[string]uint32{},
			roles:       map[string]*indexSet{},
		}
		actual, err := r.role("customRole/custom1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(IndexSetFromSlice([]uint32{0, 1, 2, 3, 4})))
		actual, err = r.role("customRole/custom2")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(IndexSetFromSlice([]uint32{1, 2, 3, 4})))
		actual, err = r.role("customRole/custom3")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(IndexSetFromSlice([]uint32{2, 3, 4})))
		perms, mapping := r.sortedPermissions()
		assert.Loosely(t, perms, should.Match([]string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3", "luci.dev.p4", "luci.dev.p5"}))
		assert.Loosely(t, mapping, should.Match([]uint32{0, 3, 1, 4, 2}))
		reMap := func(perms []string, mapping []uint32, permSet []uint32) []string {
			res := make([]string, 0, len(permSet))
			for _, idx := range permSet {
				res = append(res, perms[mapping[idx]])
			}
			return res
		}
		// This test is a bit redundant but just to ensure the permissions since
		// eyeballing the numbers is difficult.
		permSet, err := r.role("customRole/custom1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, reMap(perms, mapping, permSet.toSortedSlice()), should.Match([]string{
			"luci.dev.p1",
			"luci.dev.p4",
			"luci.dev.p2",
			"luci.dev.p5",
			"luci.dev.p3",
		}))
		permSet, err = r.role("customRole/custom2")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, reMap(perms, mapping, permSet.toSortedSlice()), should.Match([]string{
			"luci.dev.p4",
			"luci.dev.p2",
			"luci.dev.p5",
			"luci.dev.p3",
		}))
		permSet, err = r.role("customRole/custom3")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, reMap(perms, mapping, permSet.toSortedSlice()), should.Match([]string{
			"luci.dev.p2",
			"luci.dev.p5",
			"luci.dev.p3",
		}))
	})
}

func TestRealmsExpander(t *testing.T) {
	t.Parallel()

	ftt.Run("test perPrincipalBindings", t, func(t *ftt.Test) {
		t.Run("errors", func(t *ftt.Test) {
			t.Run("realm not found", func(t *ftt.Test) {
				r := &RealmsExpander{}
				_, err := r.perPrincipalBindings("test")
				assert.Loosely(t, err, should.ErrLike("realm test not found in RealmsExpander"))
			})

			t.Run("parent not found", func(t *ftt.Test) {
				r := &RealmsExpander{
					realms: map[string]*realmsconf.Realm{
						"test": {
							Name:    "test",
							Extends: []string{"test-2"},
						},
					},
				}

				_, err := r.perPrincipalBindings("test")
				assert.Loosely(t, err, should.ErrLike("failed when getting parent bindings"))
			})

			t.Run("realm name mismatch", func(t *ftt.Test) {
				r := &RealmsExpander{
					realms: map[string]*realmsconf.Realm{
						"test": {
							Name: "not-test",
						},
					},
				}
				_, err := r.perPrincipalBindings("test")
				assert.Loosely(t, err, should.ErrLike("given realm: test does not match name found internally: not-test"))
			})

			t.Run("permissions fetch issue (ErrRoleNotfound)", func(t *ftt.Test) {
				r := &RealmsExpander{
					rolesExpander: &RolesExpander{
						permissions:  map[string]uint32{},
						builtinRoles: map[string]*permissions.Role{},
						roles:        map[string]*indexSet{},
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
				_, err := r.perPrincipalBindings("test")
				assert.Loosely(t, err, should.ErrLike("there was an issue fetching permissions"))
			})
		})
	})
}
