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
	"context"
	"testing"

	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/testsupport"
)

func TestRealmsExpansion(t *testing.T) {
	t.Parallel()

	binding := func(roleName string, principals []string, restrictions map[string][]string) *realmsconf.Binding {
		conds := []*realmsconf.Condition{}
		for attr, vals := range restrictions {
			conds = append(conds, &realmsconf.Condition{
				Op: &realmsconf.Condition_Restrict{
					Restrict: &realmsconf.Condition_AttributeRestriction{
						Attribute: attr,
						Values:    vals,
					},
				},
			})
		}
		return &realmsconf.Binding{
			Role:       roleName,
			Principals: principals,
			Conditions: conds,
		}
	}

	ftt.Run("ExpandRealms works", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("completely empty", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", nil)

			expectedRealms := &protocol.Realms{
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
					},
				},
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("invalid project config", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			_, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				CustomRoles: []*realmsconf.CustomRole{
					{Name: "role/notPrefixedCorrectly"},
				},
			})
			assert.Loosely(t, err, should.ErrLike("invalid realms config"))
		})

		t.Run("empty realm", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name: "r2",
					},
					{
						Name: "r1",
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
					},
					{
						Name: "p:r1",
					},
					{
						Name: "p:r2",
					},
				},
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("simple bindings", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name: "r",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.a", []string{"group:gr1", "group:gr3"}, nil),
							binding("role/dev.b", []string{"group:gr2", "group:gr3"}, nil),
							binding("role/dev.all", []string{"group:gr4"}, nil),
						},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Permissions: []*protocol.Permission{
					{Name: "luci.dev.p1"},
					{Name: "luci.dev.p2"},
					{Name: "luci.dev.p3"},
				},
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
					},
					{
						Name: "p:r",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr3", "group:gr4"},
							},
							{
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr2"},
							},
						},
					},
				},
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("simple bindings with conditions", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name: "r",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.a", []string{"group:gr1", "group:gr3"}, nil),
							binding("role/dev.b", []string{"group:gr2", "group:gr3"}, nil),
							binding("role/dev.all", []string{"group:gr4"}, nil),
							binding("role/dev.a", []string{"group:gr1"}, map[string][]string{"a1": {"1", "2"}}),
							binding("role/dev.a", []string{"group:gr1"}, map[string][]string{"a1": {"1", "2"}}),
							binding("role/dev.a", []string{"group:gr2"}, map[string][]string{"a1": {"2", "1"}}),
							binding("role/dev.b", []string{"group:gr2"}, map[string][]string{"a1": {"1", "2"}}),
							binding("role/dev.b", []string{"group:gr2"}, map[string][]string{"a2": {"1", "2"}}),
						},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Permissions: []*protocol.Permission{
					{Name: "luci.dev.p1"},
					{Name: "luci.dev.p2"},
					{Name: "luci.dev.p3"},
				},
				Conditions: []*protocol.Condition{
					{
						Op: &protocol.Condition_Restrict{
							Restrict: &protocol.Condition_AttributeRestriction{
								Attribute: "a1",
								Values:    []string{"1", "2"},
							},
						},
					},
					{
						Op: &protocol.Condition_Restrict{
							Restrict: &protocol.Condition_AttributeRestriction{
								Attribute: "a2",
								Values:    []string{"1", "2"},
							},
						},
					},
				},
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
					},
					{
						Name: "p:r",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1"},
								Conditions:  []uint32{0},
							},
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr3", "group:gr4"},
							},
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr2"},
								Conditions:  []uint32{0},
							},
							{
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr2"},
							},
							{
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr2"},
								Conditions:  []uint32{1},
							},
						},
					},
				},
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("custom root", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name: "@root",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.all", []string{"group:gr4"}, nil),
						},
					},
					{
						Name: "r",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.a", []string{"group:gr1", "group:gr3"}, nil),
							binding("role/dev.b", []string{"group:gr2", "group:gr3"}, nil),
						},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Permissions: []*protocol.Permission{
					{Name: "luci.dev.p1"},
					{Name: "luci.dev.p2"},
					{Name: "luci.dev.p3"},
				},
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr4"},
							},
						},
					},
					{
						Name: "p:r",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr3", "group:gr4"},
							},
							{
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr2"},
							},
						},
					},
				},
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("realm inheritance", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name: "@root",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.all", []string{"group:gr4"}, nil),
						},
					},
					{
						Name: "r1",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.a", []string{"group:gr1", "group:gr3"}, nil),
						},
					},
					{
						Name: "r2",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.b", []string{"group:gr2", "group:gr3"}, nil),
						},
						Extends: []string{
							"r1",
							"@root",
						},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Permissions: []*protocol.Permission{
					{Name: "luci.dev.p1"},
					{Name: "luci.dev.p2"},
					{Name: "luci.dev.p3"},
				},
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr4"},
							},
						},
					},
					{
						Name: "p:r1",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1", "group:gr3"},
							},
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr4"},
							},
						},
					},
					{
						Name: "p:r2",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr3", "group:gr4"},
							},
							{
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr2"},
							},
						},
					},
				},
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("realm inheritance with conditions", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name: "@root",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.all", []string{"group:gr4"}, nil),
							binding("role/dev.a", []string{"group:gr5"}, map[string][]string{"a1": {"1"}}),
						},
					},
					{
						Name: "r1",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.a", []string{"group:gr1", "group:gr3"}, nil),
							binding("role/dev.a", []string{"group:gr6"}, map[string][]string{"a1": {"1"}}),
						},
					},
					{
						Name: "r2",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.b", []string{"group:gr2", "group:gr3"}, nil),
							binding("role/dev.a", []string{"group:gr1", "group:gr6", "group:gr7"}, map[string][]string{"a1": {"1"}}),
						},
						Extends: []string{
							"r1",
							"@root",
						},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Permissions: []*protocol.Permission{
					{Name: "luci.dev.p1"},
					{Name: "luci.dev.p2"},
					{Name: "luci.dev.p3"},
				},
				Conditions: []*protocol.Condition{
					{
						Op: &protocol.Condition_Restrict{
							Restrict: &protocol.Condition_AttributeRestriction{
								Attribute: "a1",
								Values:    []string{"1"},
							},
						},
					},
				},
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr5"},
								Conditions:  []uint32{0},
							},
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr4"},
							},
						},
					},
					{
						Name: "p:r1",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1", "group:gr3"},
							},
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr5", "group:gr6"},
								Conditions:  []uint32{0},
							},
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr4"},
							},
						},
					},
					{
						Name: "p:r2",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr1"},
							},
							{
								Permissions: []uint32{0, 1},
								Principals: []string{
									"group:gr1",
									"group:gr5",
									"group:gr6",
									"group:gr7",
								},
								Conditions: []uint32{0},
							},
							{
								Permissions: []uint32{0, 1, 2},
								Principals:  []string{"group:gr3", "group:gr4"},
							},
							{
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr2"},
							},
						},
					},
				},
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("custom roles", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				CustomRoles: []*realmsconf.CustomRole{
					{
						Name:        "customRole/r1",
						Extends:     []string{"role/dev.a"},
						Permissions: []string{"luci.dev.p4"},
					},
					{
						Name:    "customRole/r2",
						Extends: []string{"customRole/r1", "role/dev.b"},
					},
					{
						Name:        "customRole/r3",
						Permissions: []string{"luci.dev.p5"},
					},
				},
				Realms: []*realmsconf.Realm{
					{
						Name: "r",
						Bindings: []*realmsconf.Binding{
							binding("customRole/r1", []string{"group:gr1", "group:gr3"}, nil),
							binding("customRole/r2", []string{"group:gr2", "group:gr3"}, nil),
							binding("customRole/r3", []string{"group:gr5"}, nil),
						},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Permissions: []*protocol.Permission{
					{Name: "luci.dev.p1"},
					{Name: "luci.dev.p2"},
					{Name: "luci.dev.p3"},
					{Name: "luci.dev.p4"},
					{Name: "luci.dev.p5"},
				},
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
					},
					{
						Name: "p:r",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1, 2, 3},
								Principals:  []string{"group:gr2", "group:gr3"},
							},
							{
								Permissions: []uint32{0, 1, 3},
								Principals:  []string{"group:gr1"},
							},
							{
								Permissions: []uint32{4},
								Principals:  []string{"group:gr5"},
							},
						},
					},
				},
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("implicit root bindings with no root", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(true)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name: "r",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.a", []string{"group:gr"}, nil),
						},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Conditions: []*protocol.Condition{
					{
						Op: &protocol.Condition_Restrict{
							Restrict: &protocol.Condition_AttributeRestriction{
								Attribute: "root",
								Values:    []string{"yes"},
							},
						},
					},
				},
				Permissions: []*protocol.Permission{
					{Name: "luci.dev.implicitRoot"},
					{Name: "luci.dev.p1"},
					{Name: "luci.dev.p2"},
				},
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Principals:  []string{"project:p"},
							},
							{
								Permissions: []uint32{0},
								Conditions:  []uint32{0},
								Principals:  []string{"group:root"},
							},
						},
					},
					{
						Name: "p:r",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Principals:  []string{"project:p"},
							},
							{
								Permissions: []uint32{0},
								Conditions:  []uint32{0},
								Principals:  []string{"group:root"},
							},
							{
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr"},
							},
						},
					},
				},
			}

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("implicit root bindings with root", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(true)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name: "@root",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.a", []string{"group:gr1"}, nil),
						},
					},
					{
						Name: "r",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.a", []string{"group:gr2"}, nil),
							binding("role/dev.a", []string{"group:gr2"}, map[string][]string{"root": {"yes"}}),
							binding("role/dev.a", []string{"group:gr3"}, map[string][]string{"a1": {"1"}}),
						},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Conditions: []*protocol.Condition{
					{
						Op: &protocol.Condition_Restrict{
							Restrict: &protocol.Condition_AttributeRestriction{
								Attribute: "a1",
								Values:    []string{"1"},
							},
						},
					},
					{
						Op: &protocol.Condition_Restrict{
							Restrict: &protocol.Condition_AttributeRestriction{
								Attribute: "root",
								Values:    []string{"yes"},
							},
						},
					},
				},
				Permissions: []*protocol.Permission{
					{Name: "luci.dev.implicitRoot"},
					{Name: "luci.dev.p1"},
					{Name: "luci.dev.p2"},
				},
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Principals:  []string{"project:p"},
							},
							{
								Conditions:  []uint32{1},
								Permissions: []uint32{0},
								Principals:  []string{"group:root"},
							},
							{
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr1"},
							},
						},
					},
					{
						Name: "p:r",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0},
								Principals:  []string{"project:p"},
							},
							{
								Conditions:  []uint32{1},
								Permissions: []uint32{0},
								Principals:  []string{"group:root"},
							},
							{
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr1", "group:gr2"},
							},
							{
								Conditions:  []uint32{0},
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr3"},
							},
							{
								Conditions:  []uint32{1},
								Permissions: []uint32{1, 2},
								Principals:  []string{"group:gr2"},
							},
						},
					},
				},
			}

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("implicit root bindings in internal", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(true)
			actualRealms, err := ExpandRealms(ctx, permDB, "@internal", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name: "r",
						Bindings: []*realmsconf.Binding{
							binding("role/dev.a", []string{"group:gr"}, nil),
						},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Permissions: []*protocol.Permission{
					{Name: "luci.dev.p1", Internal: true},
					{Name: "luci.dev.p2", Internal: true},
				},
				Realms: []*protocol.Realm{
					{
						Name: "@internal:@root",
					},
					{
						Name: "@internal:r",
						Bindings: []*protocol.Binding{
							{
								Permissions: []uint32{0, 1},
								Principals:  []string{"group:gr"},
							},
						},
					},
				},
			}

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})

		t.Run("enforce in service", func(t *ftt.Test) {
			permDB := testsupport.PermissionsDB(false)
			actualRealms, err := ExpandRealms(ctx, permDB, "p", &realmsconf.RealmsCfg{
				Realms: []*realmsconf.Realm{
					{
						Name:             "@root",
						EnforceInService: []string{"a"},
					},
					{
						Name: "r1",
					},
					{
						Name:             "r2",
						EnforceInService: []string{"b"},
					},
					{
						Name:             "r3",
						EnforceInService: []string{"c"},
					},
					{
						Name:             "r4",
						Extends:          []string{"r1", "r2", "r3"},
						EnforceInService: []string{"d"},
					},
				},
			})

			expectedRealms := &protocol.Realms{
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
						Data: &protocol.RealmData{
							EnforceInService: []string{"a"},
						},
					},
					{
						Name: "p:r1",
						Data: &protocol.RealmData{
							EnforceInService: []string{"a"},
						},
					},
					{
						Name: "p:r2",
						Data: &protocol.RealmData{
							EnforceInService: []string{"a", "b"},
						},
					},
					{
						Name: "p:r3",
						Data: &protocol.RealmData{
							EnforceInService: []string{"a", "c"},
						},
					},
					{
						Name: "p:r4",
						Data: &protocol.RealmData{
							EnforceInService: []string{"a", "b", "c", "d"},
						},
					},
				},
			}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualRealms, should.Match(expectedRealms))
		})
	})
}

func TestFetchLatestRealmsConfigs(t *testing.T) {
	t.Parallel()

	ftt.Run("fetching works", t, func(t *ftt.Test) {
		testConfigClient := &testsupport.FakeConfigClient{}
		ctx := memory.Use(context.Background())
		ctx = cfgclient.Use(ctx, testConfigClient)

		latestConfigs, err := FetchLatestRealmsConfigs(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, latestConfigs, should.Match(
			testConfigClient.GetExpectedConfigsForTest(ctx)))
	})
}
