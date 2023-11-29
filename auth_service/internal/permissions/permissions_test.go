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
package permissions

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/common/data/stringset"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config"
	protocol "go.chromium.org/luci/server/auth/service/protocol"
)

func TestPermissionsDBGeneration(t *testing.T) {
	t.Parallel()
	Convey("testing permissionsDB generation", t, func() {
		attrs := []string{"attr-A", "attr-B"}
		cfg := &configspb.PermissionsConfig{
			Role: []*configspb.PermissionsConfig_Role{
				{
					Name: "role/test.reader",
					Permissions: []*protocol.Permission{
						{
							Name: "test.config.get",
						},
					},
				},
				{
					Name: "role/test.writer",
					Permissions: []*protocol.Permission{
						{
							Name: "test.config.create",
						},
					},
				},
				{
					Name: "role/supertest.writer",
					Permissions: []*protocol.Permission{
						{
							Name: "test.superconfig.create",
						},
					},
					Includes: []string{
						"role/test.writer",
					},
				},
			},
			Attribute: attrs,
		}
		expectedDB := &PermissionsDB{
			Rev: fmt.Sprintf("permissionsDB:%d", 123),
			Permissions: map[string]*protocol.Permission{
				"test.config.get":         {Name: "test.config.get"},
				"test.config.create":      {Name: "test.config.create"},
				"test.superconfig.create": {Name: "test.superconfig.create"},
			},
			Roles: map[string]*Role{
				"role/test.reader": {
					Name: "role/test.reader",
					Permissions: stringset.NewFromSlice(
						"test.config.get",
					),
				},
				"role/test.writer": {
					Name: "role/test.writer",
					Permissions: stringset.NewFromSlice(
						"test.config.create",
					),
				},
				"role/supertest.writer": {
					Name: "role/supertest.writer",
					Permissions: stringset.NewFromSlice(
						"test.superconfig.create",
						"test.config.create",
					),
				},
			},
			attributes: stringset.NewFromSlice(attrs...),
			ImplicitRootBindings: func(projID string) []*realmsconf.Binding {
				return []*realmsconf.Binding{
					{
						Role:       "role/luci.internal.system",
						Principals: []string{fmt.Sprintf("project:%s", projID)},
					},
					{
						Role:       "role/luci.internal.buildbucket.reader",
						Principals: []string{"group:buildbucket-internal-readers"},
					},
					{
						Role:       "role/luci.internal.resultdb.reader",
						Principals: []string{"group:resultdb-internal-readers"},
					},
					{
						Role:       "role/luci.internal.resultdb.invocationSubmittedSetter",
						Principals: []string{"group:resultdb-internal-invocation-submitters"},
					},
				}
			},
		}

		Convey("succeeds without permissions config", func() {
			db := NewPermissionsDB(nil, nil)
			So(db.Rev, ShouldEqual, "config-without-metadata")
			So(db.Permissions, ShouldBeEmpty)
			So(db.Roles, ShouldBeEmpty)
			So(db.attributes, ShouldBeEmpty)
		})

		Convey("succeeds without metadata", func() {
			db := NewPermissionsDB(cfg, nil)
			So(db.Rev, ShouldEqual, "config-without-metadata")
			So(db.Permissions, ShouldResemble, expectedDB.Permissions)
			So(db.Roles, ShouldResemble, expectedDB.Roles)
			So(setsAreEqual(db.attributes, expectedDB.attributes), ShouldBeTrue)
		})

		Convey("has metadata info", func() {
			db := NewPermissionsDB(cfg, &config.Meta{
				Path:     "permissions.cfg",
				Revision: "123",
			})
			So(db.Rev, ShouldEqual, "permissions.cfg:123")
			So(db.Permissions, ShouldResemble, expectedDB.Permissions)
			So(db.Roles, ShouldResemble, expectedDB.Roles)
			So(setsAreEqual(db.attributes, expectedDB.attributes), ShouldBeTrue)
		})
	})
}

func setsAreEqual(a, b stringset.Set) bool {
	return a.Contains(b) && b.Contains(a)
}
