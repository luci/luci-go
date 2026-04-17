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
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/configspb"
)

func TestPermissionsDBGeneration(t *testing.T) {
	t.Parallel()

	cfg := &configspb.PermissionsConfig{
		Permission: []*protocol.Permission{
			{Name: "test.config.get", Attributes: []string{"a", "b"}},
			{Name: "test.config.create"},
			{Name: "test.superconfig.create", Attributes: []string{"b", "c"}},
		},
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
				Name: "role/supertest.writer",
				Permissions: []*protocol.Permission{
					{
						Name: "test.superconfig.create",
					},
				},
				Includes: []string{
					"role/test.writer",
					"role/test.reader",
				},
			},
			{
				Name: "role/test.writer",
				Permissions: []*protocol.Permission{
					{
						Name: "test.config.create",
					},
				},
				Includes: []string{
					"role/test.reader",
				},
			},
		},
	}

	expectedDB := &PermissionsDB{
		Rev: fmt.Sprintf("permissionsDB:%d", 123),
		Permissions: map[string]*protocol.Permission{
			"test.config.get": {
				Name:       "test.config.get",
				Attributes: []string{"a", "b"},
			},
			"test.config.create": {
				Name: "test.config.create",
			},
			"test.superconfig.create": {
				Name:       "test.superconfig.create",
				Attributes: []string{"b", "c"},
			},
		},
		Roles: map[string]*Role{
			"role/test.reader": {
				Name: "role/test.reader",
				Permissions: stringset.NewFromSlice(
					"test.config.get",
				),
				RelevantAttributes: stringset.NewFromSlice("a", "b"),
			},
			"role/test.writer": {
				Name: "role/test.writer",
				Permissions: stringset.NewFromSlice(
					"test.config.create",
					"test.config.get",
				),
				Includes: []string{
					"role/test.reader",
				},
				RelevantAttributes: stringset.NewFromSlice("a", "b"),
			},
			"role/supertest.writer": {
				Name: "role/supertest.writer",
				Permissions: stringset.NewFromSlice(
					"test.superconfig.create",
					"test.config.create",
					"test.config.get",
				),
				Includes: []string{
					"role/test.writer",
					"role/test.reader",
				},
				RelevantAttributes: stringset.NewFromSlice("a", "b", "c"),
			},
		},
	}

	t.Run("succeeds without permissions config", func(t *testing.T) {
		db := NewPermissionsDB(nil, nil)
		assert.Loosely(t, db.Rev, should.Equal("config-without-metadata"))
		assert.Loosely(t, db.Permissions, should.BeEmpty)
		assert.Loosely(t, db.Roles, should.BeEmpty)
	})

	t.Run("works", func(t *testing.T) {
		db := NewPermissionsDB(cfg, &config.Meta{
			Path:     "permissions.cfg",
			Revision: "123",
		})
		assert.Loosely(t, db.Rev, should.Equal("permissions.cfg:123"))
		assert.Loosely(t, db.Permissions, should.Match(expectedDB.Permissions))
		assert.Loosely(t, db.Roles, should.Match(expectedDB.Roles))
	})
}
