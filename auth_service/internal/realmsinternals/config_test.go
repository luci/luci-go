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
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/filter/txndefer"
	gaemem "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	testModifiedTS = time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC)
)

func testAuthVersionedEntityMixin() model.AuthVersionedEntityMixin {
	return model.AuthVersionedEntityMixin{
		ModifiedTS:    testModifiedTS,
		ModifiedBy:    "user:test-modifier@example.com",
		AuthDBRev:     1337,
		AuthDBPrevRev: 1336,
	}
}

func TestGetConfigs(t *testing.T) {
	projectRealmsKey := func(ctx context.Context, project string) *datastore.Key {
		return datastore.NewKey(ctx, "AuthProjectRealms", project, 0, model.RootKey(ctx))
	}

	Convey("projects with config", t, func() {
		ctx := gaemem.Use(context.Background())
		ctx = cfgclient.Use(ctx, &fakeCfgClient{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)
		datastore.Put(ctx, &model.AuthProjectRealms{
			AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
			Kind:                     "AuthProjectRealms",
			ID:                       "test",
			Parent:                   model.RootKey(ctx),
			Realms:                   []byte{},
			PermsRev:                 "123",
			ConfigRev:                "1234",
		})
		datastore.Put(ctx, &model.AuthProjectRealmsMeta{
			Kind:         "AuthProjectRealmsMeta",
			ID:           "meta",
			Parent:       projectRealmsKey(ctx, "test"),
			PermsRev:     "123",
			ConfigRev:    "1234",
			ConfigDigest: "test-digest",
			ModifiedTS:   testModifiedTS,
		})

		latestExpected := []*RealmsCfgRev{
			{
				projectID:    "@internal",
				configRev:    testRevision,
				configDigest: testContentHash,
				configBody:   []byte{},
				permsRev:     "",
			},
			{
				projectID:    "test-project-a",
				configRev:    testRevision,
				configDigest: testContentHash,
				configBody:   []byte{},
				permsRev:     "",
			},

			{
				projectID:    "test-project-d",
				configRev:    testRevision,
				configDigest: testContentHash,
				configBody:   []byte{},
				permsRev:     "",
			},
		}

		storedExpected := []*RealmsCfgRev{
			{
				projectID:    "meta",
				configRev:    "1234",
				permsRev:     "123",
				configDigest: "test-digest",
			},
		}

		latest, stored, err := GetConfigs(ctx)

		sortRevsByID(latest)
		sortRevsByID(latestExpected)

		So(err, ShouldBeNil)
		So(latest, ShouldResemble, latestExpected)
		So(stored, ShouldResemble, storedExpected)
	})

	Convey("no projects with config", t, func() {
		ctx := gaemem.Use(context.Background())
		ctx = cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{}))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = txndefer.FilterRDS(ctx)
		datastore.Put(ctx, &model.AuthProjectRealms{
			AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
			Kind:                     "AuthProjectRealms",
			ID:                       "test",
			Parent:                   model.RootKey(ctx),
			Realms:                   []byte{},
			PermsRev:                 "123",
			ConfigRev:                "1234",
		})
		datastore.Put(ctx, &model.AuthProjectRealmsMeta{
			Kind:         "AuthProjectRealmsMeta",
			ID:           "meta",
			Parent:       projectRealmsKey(ctx, "test"),
			PermsRev:     "123",
			ConfigRev:    "1234",
			ConfigDigest: "test-digest",
			ModifiedTS:   testModifiedTS,
		})
		_, _, err := GetConfigs(ctx)
		So(err, ShouldErrLike, config.ErrNoConfig)
	})

	Convey("no entity in ds", t, func() {
		ctx := gaemem.Use(context.Background())
		ctx = cfgclient.Use(ctx, &fakeCfgClient{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		_, stored, err := GetConfigs(ctx)
		So(err, ShouldBeNil)
		So(stored, ShouldHaveLength, 0)
	})

}

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

	Convey("ExpandRealms works", t, func() {
		Convey("completely empty", func() {
			permDB := testPermissionsDB(false)
			actualRealms, err := ExpandRealms(permDB, "p", nil)

			expectedRealms := &protocol.Realms{
				Realms: []*protocol.Realm{
					{
						Name: "p:@root",
					},
				},
			}
			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("empty realm", func() {
			permDB := testPermissionsDB(false)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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
			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("simple bindings", func() {
			permDB := testPermissionsDB(false)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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
			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("simple bindings with conditions", func() {
			permDB := testPermissionsDB(false)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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
			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("custom root", func() {
			permDB := testPermissionsDB(false)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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
			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("realm inheritance", func() {
			permDB := testPermissionsDB(false)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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
			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("realm inheritance with conditions", func() {
			permDB := testPermissionsDB(false)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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
			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("custom roles", func() {
			permDB := testPermissionsDB(false)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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
			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("implicit root bindings with no root", func() {
			permDB := testPermissionsDB(true)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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

			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("implicit root bindings with root", func() {
			permDB := testPermissionsDB(true)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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

			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("implicit root bindings in internal", func() {
			permDB := testPermissionsDB(true)
			actualRealms, err := ExpandRealms(permDB, "@internal", &realmsconf.RealmsCfg{
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

			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})

		Convey("enforce in service", func() {
			permDB := testPermissionsDB(false)
			actualRealms, err := ExpandRealms(permDB, "p", &realmsconf.RealmsCfg{
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
			So(err, ShouldBeNil)
			So(actualRealms, ShouldResembleProto, expectedRealms)
		})
	})
}

func sortRevsByID(revs []*RealmsCfgRev) {
	sort.Slice(revs, func(i, j int) bool {
		return revs[i].projectID < revs[j].projectID
	})
}
