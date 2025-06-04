// Copyright 2025 The LUCI Authors.
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

package bqexport

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/bqpb"
	"go.chromium.org/luci/auth_service/api/configspb"
)

func TestAnalyzePermissionsCfg(t *testing.T) {
	t.Parallel()

	ftt.Run("returns roles", t, func(t *ftt.Test) {
		ctx := context.Background()
		permsURL := "https://path.to.view/config/permissions.cfg/123abc"
		testPermsCfg := &configspb.PermissionsConfig{
			Role: []*configspb.PermissionsConfig_Role{
				{
					Name: "role/test.service.reader",
					Permissions: []*protocol.Permission{
						{
							Name: "test.service.get",
						},
						{
							Name: "test.service.list",
						},
					},
				},
				{
					Name: "role/test.service.writer",
					Permissions: []*protocol.Permission{
						{
							Name: "test.service.write",
						},
						{
							Name: "test.service.create",
						},
					},
				},
				{
					Name: "role/test.service.admin",
					Permissions: []*protocol.Permission{
						{
							Name: "test.service.delete",
						},
					},
					Includes: []string{
						"role/test.service.reader",
						"role/test.service.writer",
					},
				},
			},
		}

		perms, roles := analyzePermissionsCfg(ctx, testPermsCfg, permsURL)
		assert.Loosely(t, perms, should.Match(stringset.NewFromSlice(
			"test.service.get", "test.service.list", "test.service.write",
			"test.service.create", "test.service.delete")))
		assert.Loosely(t, roles, should.Match(RoleSet{
			"role/test.service.reader": {
				Name:     "role/test.service.reader",
				Subroles: stringset.Set{},
				Permissions: stringset.NewFromSlice(
					"test.service.get", "test.service.list",
				),
				ViewURL: permsURL,
			},
			"role/test.service.writer": {
				Name:     "role/test.service.writer",
				Subroles: stringset.Set{},
				Permissions: stringset.NewFromSlice(
					"test.service.write", "test.service.create",
				),
				ViewURL: permsURL,
			},
			"role/test.service.admin": {
				Name: "role/test.service.admin",
				Subroles: stringset.NewFromSlice(
					"role/test.service.writer", "role/test.service.reader",
				),
				Permissions: stringset.NewFromSlice(
					"test.service.get", "test.service.list", "test.service.write",
					"test.service.create", "test.service.delete",
				),
				ViewURL: permsURL,
			},
		}))
	})
}

func TestAnalyzeRealmsCfg(t *testing.T) {
	t.Parallel()

	ftt.Run("analyzeRealmsCfgRoles works", t, func(t *ftt.Test) {
		ctx := context.Background()
		permsURL := "https://path.to.view/config/permissions.cfg/123abc"
		testPerms := stringset.NewFromSlice(
			"test.service.get", "test.service.list", "test.service.write",
			"test.service.create", "test.service.delete",
		)
		testGlobalRoles := RoleSet{
			"role/test.service.reader": {
				Name:     "role/test.service.reader",
				Subroles: stringset.Set{},
				Permissions: stringset.NewFromSlice(
					"test.service.get", "test.service.list",
				),
				ViewURL: permsURL,
			},
			"role/test.service.writer": {
				Name:     "role/test.service.writer",
				Subroles: stringset.Set{},
				Permissions: stringset.NewFromSlice(
					"test.service.write", "test.service.create",
				),
				ViewURL: permsURL,
			},
			"role/test.service.admin": {
				Name: "role/test.service.admin",
				Subroles: stringset.NewFromSlice(
					"role/test.service.writer", "role/test.service.reader",
				),
				Permissions: stringset.NewFromSlice(
					"test.service.get", "test.service.list", "test.service.write",
					"test.service.create", "test.service.delete",
				),
				ViewURL: permsURL,
			},
		}
		testRealmsURL := "https://path.to.view/config/project-a/realms.cfg/foo"
		testRealmsCfg := &realmsconf.RealmsCfg{
			CustomRoles: []*realmsconf.CustomRole{
				{
					Name:        "customRole/projectA.limitedReader",
					Permissions: []string{"test.service.list"},
				},
				{
					Name:        "customRole/projectA.denied",
					Permissions: []string{},
				},
				{
					Name: "customRole/projectA.admin",
					Extends: []string{
						"role/test.service.reader", "role/test.service.writer",
					},
				},
			},
		}

		actual := analyzeRealmsCfgRoles(ctx, testRealmsCfg, testRealmsURL, testPerms, testGlobalRoles)
		assert.Loosely(t, actual, should.Match(RoleSet{
			"customRole/projectA.limitedReader": {
				Name:        "customRole/projectA.limitedReader",
				Subroles:    stringset.Set{},
				Permissions: stringset.NewFromSlice("test.service.list"),
				ViewURL:     testRealmsURL,
			},
			"customRole/projectA.denied": {
				Name:        "customRole/projectA.denied",
				Subroles:    stringset.Set{},
				Permissions: stringset.Set{},
				ViewURL:     testRealmsURL,
			},
			"customRole/projectA.admin": {
				Name: "customRole/projectA.admin",
				Subroles: stringset.NewFromSlice(
					"role/test.service.reader", "role/test.service.writer",
				),
				Permissions: stringset.NewFromSlice(
					"test.service.get", "test.service.list", "test.service.write",
					"test.service.create",
				),
				ViewURL: testRealmsURL,
			},
		}))
	})
}

func TestCollateLatestRoles(t *testing.T) {
	t.Parallel()

	ftt.Run("collateLatestRoles works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testTime := timestamppb.New(
			time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC))

		permsURL := "https://path.to.view/config/permissions.cfg/123abc"
		testPermsCfg := &configspb.PermissionsConfig{
			Role: []*configspb.PermissionsConfig_Role{
				{
					Name: "role/test.service.reader",
					Permissions: []*protocol.Permission{
						{
							Name: "test.service.get",
						},
						{
							Name: "test.service.list",
						},
					},
				},
				{
					Name: "role/test.service.writer",
					Permissions: []*protocol.Permission{
						{
							Name: "test.service.write",
						},
						{
							Name: "test.service.create",
						},
					},
				},
				{
					Name: "role/test.service.admin",
					Permissions: []*protocol.Permission{
						{
							Name: "test.service.delete",
						},
					},
					Includes: []string{
						"role/test.service.reader",
						"role/test.service.writer",
					},
				},
			},
		}
		testLatest := &LatestConfigs{
			Permissions: &ViewableConfig[*configspb.PermissionsConfig]{
				ViewURL: permsURL,
				Config:  testPermsCfg,
			},
		}

		actual := collateLatestRoles(ctx, testLatest, testTime)
		expected := []*bqpb.RoleRow{
			{
				Name: "role/test.service.admin",
				Subroles: []string{
					"role/test.service.reader", "role/test.service.writer",
				},
				Permissions: []string{
					"test.service.create", "test.service.delete",
					"test.service.get", "test.service.list", "test.service.write",
				},
				Url:        permsURL,
				ExportedAt: testTime,
			},
			{
				Name:        "role/test.service.reader",
				Subroles:    []string{},
				Permissions: []string{"test.service.get", "test.service.list"},
				Url:         permsURL,
				ExportedAt:  testTime,
			},
			{
				Name:     "role/test.service.writer",
				Subroles: []string{},
				Permissions: []string{
					"test.service.create", "test.service.write",
				},
				Url:        permsURL,
				ExportedAt: testTime,
			},
		}
		// Sort the rows so we can easily compare to the expected value.
		slices.SortStableFunc(actual, func(a, b *bqpb.RoleRow) int {
			return strings.Compare(a.Name, b.Name)
		})
		assert.Loosely(t, actual, should.Match(expected))
	})
}
