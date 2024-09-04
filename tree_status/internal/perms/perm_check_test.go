// Copyright 2024 The LUCI Authors.
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

package perms

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/tree_status/internal/config"
)

func TestHasAccess(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	ftt.Run("has access", t, func(t *ftt.Test) {
		testConfig := config.TestConfig()
		err := config.SetConfig(ctx, testConfig)
		assert.Loosely(t, err, should.BeNil)

		// Fake the group that the user belongs to.
		ctx = authtest.MockAuthConfig(ctx)
		ctx = FakeAuth().SetInContext(ctx)

		t.Run("common", func(t *ftt.Test) {
			t.Run("tree not configured", func(t *ftt.Test) {
				allowed, message, err := hasAccess(ctx, "some-tree", "luci-tree-status-access", PermGetStatusLimited)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("tree has not been configured"))
			})
			t.Run("default acl anonymous should not have access", func(t *ftt.Test) {
				ctx = FakeAuth().Anonymous().SetInContext(ctx)
				allowed, message, err := hasAccess(ctx, "chromium", "luci-tree-status-access", PermGetStatusLimited)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("please log in for access"))
			})

			t.Run("default acl do not have access", func(t *ftt.Test) {
				ctx = FakeAuth().SetInContext(ctx)
				allowed, message, err := hasAccess(ctx, "chromium", "luci-tree-status-access", PermGetStatusLimited)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user is not a member of group \"luci-tree-status-access\""))
			})

			t.Run("default acl have access", func(t *ftt.Test) {
				ctx = FakeAuth().WithReadAccess().SetInContext(ctx)
				allowed, _, err := hasAccess(ctx, "chromium", "luci-tree-status-access", PermGetStatusLimited)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})

			t.Run("realm-based acl project not configured", func(t *ftt.Test) {
				testConfig.Trees[0].Projects = []string{}
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				allowed, message, err := hasAccess(ctx, "chromium", "luci-tree-status-access", PermGetStatusLimited)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("projects in tree has not been configured"))
			})

			t.Run("realm-based permission denied", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				allowed, message, err := hasAccess(ctx, "chromium", "luci-tree-status-access", PermGetStatusLimited)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user does not have permission to perform this action"))
			})

			t.Run("realm-based permission allowed", func(t *ftt.Test) {
				ctx = FakeAuth().WithPermissionInRealm(PermGetStatusLimited, "chromium:@project").SetInContext(ctx)
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				allowed, _, err := hasAccess(ctx, "chromium", "luci-tree-status-access", PermGetStatusLimited)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})
		})

		t.Run("has get status limited permission", func(t *ftt.Test) {
			t.Run("default acls, denied", func(t *ftt.Test) {
				allowed, message, err := HasGetStatusLimitedPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user is not a member of group \"luci-tree-status-access\""))
			})

			t.Run("default acls, allowed", func(t *ftt.Test) {
				ctx = FakeAuth().WithReadAccess().SetInContext(ctx)
				allowed, _, err := HasGetStatusLimitedPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})

			t.Run("realm-based acls, denied", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				allowed, message, err := HasGetStatusLimitedPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user does not have permission to perform this action"))
			})

			t.Run("realm-based acls, allowed", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = FakeAuth().WithPermissionInRealm(PermGetStatusLimited, "chromium:@project").SetInContext(ctx)
				allowed, _, err := HasGetStatusLimitedPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})
		})

		t.Run("has list status limited permission", func(t *ftt.Test) {
			t.Run("default acls, denied", func(t *ftt.Test) {
				allowed, message, err := HasListStatusLimitedPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user is not a member of group \"luci-tree-status-access\""))
			})

			t.Run("default acls, allowed", func(t *ftt.Test) {
				ctx = FakeAuth().WithReadAccess().SetInContext(ctx)
				allowed, _, err := HasListStatusLimitedPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})

			t.Run("realm-based acls, denied", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				allowed, message, err := HasListStatusLimitedPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user does not have permission to perform this action"))
			})

			t.Run("realm-based acls, allowed", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = FakeAuth().WithPermissionInRealm(PermListStatusLimited, "chromium:@project").SetInContext(ctx)
				allowed, _, err := HasListStatusLimitedPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})
		})

		t.Run("has get status permission", func(t *ftt.Test) {
			t.Run("default acls, denied", func(t *ftt.Test) {
				allowed, message, err := HasGetStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user is not a member of group \"luci-tree-status-audit-access\""))
			})

			t.Run("default acls, allowed", func(t *ftt.Test) {
				ctx = FakeAuth().WithAuditAccess().SetInContext(ctx)
				allowed, _, err := HasGetStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})

			t.Run("realm-based acls, denied", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				allowed, message, err := HasGetStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user does not have permission to perform this action"))
			})

			t.Run("realm-based acls, no audit access, denied", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = FakeAuth().WithPermissionInRealm(PermGetStatus, "chromium:@project").SetInContext(ctx)
				allowed, message, err := HasGetStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user is not a member of group \"luci-tree-status-audit-access\""))
			})

			t.Run("realm-based acls, allowed", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = FakeAuth().WithPermissionInRealm(PermGetStatus, "chromium:@project").WithAuditAccess().SetInContext(ctx)
				allowed, _, err := HasGetStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})
		})

		t.Run("has list status permission", func(t *ftt.Test) {
			t.Run("default acls, denied", func(t *ftt.Test) {
				allowed, message, err := HasListStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user is not a member of group \"luci-tree-status-audit-access\""))
			})

			t.Run("default acls, allowed", func(t *ftt.Test) {
				ctx = FakeAuth().WithAuditAccess().SetInContext(ctx)
				allowed, _, err := HasListStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})

			t.Run("realm-based acls, denied", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				allowed, message, err := HasListStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user does not have permission to perform this action"))
			})

			t.Run("realm-based acls, no audit access, denied", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = FakeAuth().WithPermissionInRealm(PermListStatus, "chromium:@project").SetInContext(ctx)
				allowed, message, err := HasListStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user is not a member of group \"luci-tree-status-audit-access\""))
			})

			t.Run("realm-based acls, allowed", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = FakeAuth().WithPermissionInRealm(PermListStatus, "chromium:@project").WithAuditAccess().SetInContext(ctx)
				allowed, _, err := HasListStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})
		})

		t.Run("has create status permission", func(t *ftt.Test) {
			t.Run("default acls, denied", func(t *ftt.Test) {
				allowed, message, err := HasCreateStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user is not a member of group \"luci-tree-status-writers\""))
			})

			t.Run("default acls, allowed", func(t *ftt.Test) {
				ctx = FakeAuth().WithWriteAccess().SetInContext(ctx)
				allowed, _, err := HasCreateStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})

			t.Run("realm-based acls, denied", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				allowed, message, err := HasCreateStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user does not have permission to perform this action"))
			})

			t.Run("realm-based acls, allowed", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = FakeAuth().WithPermissionInRealm(PermCreateStatus, "chromium:@project").SetInContext(ctx)
				allowed, _, err := HasCreateStatusPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})
		})

		t.Run("has query trees permission", func(t *ftt.Test) {
			t.Run("default acls, denied", func(t *ftt.Test) {
				allowed, message, err := HasQueryTreesPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user is not a member of group \"luci-tree-status-access\""))
			})

			t.Run("default acls, allowed", func(t *ftt.Test) {
				ctx = FakeAuth().WithReadAccess().SetInContext(ctx)
				allowed, _, err := HasQueryTreesPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})

			t.Run("realm-based acls, denied", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				allowed, message, err := HasQueryTreesPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeFalse)
				assert.That(t, message, should.Equal("user does not have permission to perform this action"))
			})

			t.Run("realm-based acls, allowed", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)
				ctx = FakeAuth().WithPermissionInRealm(PermListTree, "chromium:@project").SetInContext(ctx)
				allowed, _, err := HasQueryTreesPermission(ctx, "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, allowed, should.BeTrue)
			})
		})
	})

}
