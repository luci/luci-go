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

	"go.chromium.org/luci/common/data/stringset"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	protocol "go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/configspb"
)

func TestPermissionsDBGeneration(t *testing.T) {
	t.Parallel()
	ftt.Run("testing permissionsDB generation", t, func(t *ftt.Test) {
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

		t.Run("succeeds without permissions config", func(t *ftt.Test) {
			db := NewPermissionsDB(nil, nil)
			assert.Loosely(t, db.Rev, should.Equal("config-without-metadata"))
			assert.Loosely(t, db.Permissions, should.BeEmpty)
			assert.Loosely(t, db.Roles, should.BeEmpty)
			assert.Loosely(t, db.attributes, should.BeEmpty)
		})

		t.Run("succeeds without metadata", func(t *ftt.Test) {
			db := NewPermissionsDB(cfg, nil)
			assert.Loosely(t, db.Rev, should.Equal("config-without-metadata"))
			assert.Loosely(t, db.Permissions, should.Resemble(expectedDB.Permissions))
			assert.Loosely(t, db.Roles, should.Resemble(expectedDB.Roles))
			assert.Loosely(t, setsAreEqual(db.attributes, expectedDB.attributes), should.BeTrue)
		})

		t.Run("has metadata info", func(t *ftt.Test) {
			db := NewPermissionsDB(cfg, &config.Meta{
				Path:     "permissions.cfg",
				Revision: "123",
			})
			assert.Loosely(t, db.Rev, should.Equal("permissions.cfg:123"))
			assert.Loosely(t, db.Permissions, should.Resemble(expectedDB.Permissions))
			assert.Loosely(t, db.Roles, should.Resemble(expectedDB.Roles))
			assert.Loosely(t, setsAreEqual(db.attributes, expectedDB.attributes), should.BeTrue)
		})
	})
}

func setsAreEqual(a, b stringset.Set) bool {
	return a.Contains(b) && b.Contains(a)
}
