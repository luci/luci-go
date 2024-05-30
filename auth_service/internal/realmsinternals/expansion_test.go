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
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/auth_service/testsupport"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
	Convey("test key", t, func() {
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
		So(cond1Key, ShouldEqual, cond2Key)
		condEmpty := &protocol.Condition{}
		So(conditionKey(condEmpty), ShouldEqual, "")
	})
	Convey("errors", t, func() {
		cs := &ConditionsSet{
			normalized:   map[string]*conditionMapTuple{},
			indexMapping: map[*realmsconf.Condition]uint32{},
		}
		r1 := restriction("a", []string{"1", "2"})
		r2 := restriction("b", []string{"1"})
		So(cs.addCond(r1), ShouldBeNil)
		cs.finalize()
		So(cs.addCond(r2), ShouldEqual, ErrFinalized)
	})
	Convey("works", t, func() {
		cs := &ConditionsSet{
			normalized:   map[string]*conditionMapTuple{},
			indexMapping: map[*realmsconf.Condition]uint32{},
			finalized:    false,
		}
		r1 := restriction("b", []string{"1", "2"})
		r2 := restriction("a", []string{"2", "1", "1"})
		r3 := restriction("a", []string{"1", "2"})
		r4 := restriction("a", []string{"3", "4"})
		So(cs.addCond(r1), ShouldBeNil)
		So(cs.addCond(r1), ShouldBeNil)
		So(cs.addCond(r2), ShouldBeNil)
		So(cs.addCond(r3), ShouldBeNil)
		So(cs.addCond(r4), ShouldBeNil)
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
		So(out, ShouldResembleProto, expected)
		So(cs.indexes([]*realmsconf.Condition{r1}), ShouldResemble, []uint32{2})
		So(cs.indexes([]*realmsconf.Condition{r2}), ShouldResemble, []uint32{0})
		So(cs.indexes([]*realmsconf.Condition{r3}), ShouldResemble, []uint32{0})
		So(cs.indexes([]*realmsconf.Condition{r4}), ShouldResemble, []uint32{1})
		inds := cs.indexes([]*realmsconf.Condition{r1, r2, r3, r4})
		So(inds, ShouldResemble, []uint32{0, 1, 2})
	})
}
func TestRolesExpander(t *testing.T) {
	t.Parallel()
	Convey("errors", t, func() {
		permDB := testsupport.PermissionsDB(false)
		r := &RolesExpander{
			builtinRoles: permDB.Roles,
			customRoles:  map[string]*realmsconf.CustomRole{},
			permissions:  map[string]uint32{},
			roles:        map[string]*indexSet{},
		}
		_, err := r.role("role/notbuiltin")
		So(err, ShouldErrLike, ErrRoleNotFound)
		_, err = r.role("customRole/notarole")
		So(err, ShouldErrLike, ErrRoleNotFound)
		_, err = r.role("notarole/test")
		So(err, ShouldErrLike, ErrImpossibleRole)
	})
	Convey("test builtin roles works", t, func() {
		permDB := testsupport.PermissionsDB(false)
		r := &RolesExpander{
			builtinRoles: permDB.Roles,
			permissions:  map[string]uint32{},
			roles:        map[string]*indexSet{},
		}
		actual, err := r.role("role/dev.a")
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, IndexSetFromSlice([]uint32{0, 1}))
		actual, err = r.role("role/dev.b")
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, IndexSetFromSlice([]uint32{1, 2}))
		perms, mapping := r.sortedPermissions()
		So(perms, ShouldResemble, []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"})
		So(mapping, ShouldResemble, []uint32{0, 1, 2})
	})
	Convey("test custom roles works", t, func() {
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
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, IndexSetFromSlice([]uint32{0, 1, 2, 3, 4}))
		actual, err = r.role("customRole/custom2")
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, IndexSetFromSlice([]uint32{1, 2, 3, 4}))
		actual, err = r.role("customRole/custom3")
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, IndexSetFromSlice([]uint32{2, 3, 4}))
		perms, mapping := r.sortedPermissions()
		So(perms, ShouldResemble, []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3", "luci.dev.p4", "luci.dev.p5"})
		So(mapping, ShouldResemble, []uint32{0, 3, 1, 4, 2})
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
		So(err, ShouldBeNil)
		So(reMap(perms, mapping, permSet.toSortedSlice()), ShouldResemble, []string{
			"luci.dev.p1",
			"luci.dev.p4",
			"luci.dev.p2",
			"luci.dev.p5",
			"luci.dev.p3",
		})
		permSet, err = r.role("customRole/custom2")
		So(err, ShouldBeNil)
		So(reMap(perms, mapping, permSet.toSortedSlice()), ShouldResemble, []string{
			"luci.dev.p4",
			"luci.dev.p2",
			"luci.dev.p5",
			"luci.dev.p3",
		})
		permSet, err = r.role("customRole/custom3")
		So(err, ShouldBeNil)
		So(reMap(perms, mapping, permSet.toSortedSlice()), ShouldResemble, []string{
			"luci.dev.p2",
			"luci.dev.p5",
			"luci.dev.p3",
		})
	})
}

func TestRealmsExpander(t *testing.T) {
	t.Parallel()

	Convey("test perPrincipalBindings", t, func() {
		Convey("errors", func() {
			Convey("realm not found", func() {
				r := &RealmsExpander{}
				_, err := r.perPrincipalBindings("test")
				So(err, ShouldErrLike, "realm test not found in RealmsExpander")
			})

			Convey("parent not found", func() {
				r := &RealmsExpander{
					realms: map[string]*realmsconf.Realm{
						"test": {
							Name:    "test",
							Extends: []string{"test-2"},
						},
					},
				}

				_, err := r.perPrincipalBindings("test")
				So(err, ShouldErrLike, "failed when getting parent bindings")
			})

			Convey("realm name mismatch", func() {
				r := &RealmsExpander{
					realms: map[string]*realmsconf.Realm{
						"test": {
							Name: "not-test",
						},
					},
				}
				_, err := r.perPrincipalBindings("test")
				So(err, ShouldErrLike, "given realm: test does not match name found internally: not-test")
			})

			Convey("permissions fetch issue (ErrRoleNotfound)", func() {
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
				So(err, ShouldErrLike, "there was an issue fetching permissions")
			})
		})
	})
}
