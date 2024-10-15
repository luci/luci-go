// Copyright 2023 The LUCI Authors.
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

package acl

import (
	"testing"

	"go.chromium.org/luci/auth/identity"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"
)

func TestConfigSet(t *testing.T) {
	t.Parallel()

	ftt.Run("Config Set ACL Check", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		fakeAuthDB := authtest.NewFakeDB()
		const requester = identity.Identity("user:requester@example.com")
		var projectConfigSet = config.MustProjectSet("example-project")
		var serviceConfigSet = config.MustServiceSet("my-service")
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: requester,
			FakeDB:   fakeAuthDB,
		})
		const projectAccessGroup = "project-access-group"
		const projectValidationGroup = "project-validate-group"
		const projectReimportGroup = "project-reimport-group"
		const serviceAccessGroup = "service-access-group"
		const serviceValidationGroup = "service-validate-group"
		const serviceReimportGroup = "service-reimport-group"
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ProjectAccessGroup:     projectAccessGroup,
				ProjectValidationGroup: projectValidationGroup,
				ProjectReimportGroup:   projectReimportGroup,
				ServiceAccessGroup:     serviceAccessGroup,
				ServiceValidationGroup: serviceValidationGroup,
				ServiceReimportGroup:   serviceReimportGroup,
			},
		})
		t.Run("CanReadConfigSet", func(t *ftt.Test) {
			t.Run("Project", func(t *ftt.Test) {
				t.Run("Denied", func(t *ftt.Test) {
					allowed, err := CanReadConfigSet(ctx, projectConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeFalse)
				})
				t.Run("Allowed", func(t *ftt.Test) {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, projectAccessGroup))
					allowed, err := CanReadConfigSet(ctx, projectConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeTrue)
				})
			})
			t.Run("Service", func(t *ftt.Test) {
				t.Run("Denied", func(t *ftt.Test) {
					allowed, err := CanReadConfigSet(ctx, serviceConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeFalse)
				})
				t.Run("Allowed", func(t *ftt.Test) {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, serviceAccessGroup))
					allowed, err := CanReadConfigSet(ctx, serviceConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeTrue)
				})
			})
		})

		t.Run("CanReadConfigSets", func(t *ftt.Test) {
			fakeAuthDB.AddMocks(
				authtest.MockPermission(requester, realms.Join(projectConfigSet.Project(), realms.RootRealm), ReadPermission),
				authtest.MockMembership(requester, serviceAccessGroup),
			)
			result, err := CanReadConfigSets(ctx, []config.Set{projectConfigSet, serviceConfigSet, config.MustServiceSet("another-service"), config.MustProjectSet("another-project")})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble([]bool{true, true, true, false}))
		})

		t.Run("CanValidateConfigSet", func(t *ftt.Test) {
			fakeAuthDB.AddMocks( // allow read access
				authtest.MockMembership(requester, projectAccessGroup),
				authtest.MockMembership(requester, serviceAccessGroup),
			)
			t.Run("Project", func(t *ftt.Test) {
				t.Run("Denied", func(t *ftt.Test) {
					allowed, err := CanValidateConfigSet(ctx, projectConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeFalse)
				})
				t.Run("Allowed", func(t *ftt.Test) {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, projectValidationGroup))
					allowed, err := CanValidateConfigSet(ctx, projectConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeTrue)
				})
			})
			t.Run("Service", func(t *ftt.Test) {
				t.Run("Denied", func(t *ftt.Test) {
					allowed, err := CanValidateConfigSet(ctx, serviceConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeFalse)
				})
				t.Run("Allowed", func(t *ftt.Test) {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, serviceValidationGroup))
					allowed, err := CanValidateConfigSet(ctx, serviceConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeTrue)
				})
			})
		})

		t.Run("CanReimportProject", func(t *ftt.Test) {
			fakeAuthDB.AddMocks( // allow read access
				authtest.MockMembership(requester, projectAccessGroup),
				authtest.MockMembership(requester, serviceAccessGroup),
			)
			t.Run("Project", func(t *ftt.Test) {
				t.Run("Denied", func(t *ftt.Test) {
					allowed, err := CanReimportConfigSet(ctx, projectConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeFalse)
				})
				t.Run("Allowed", func(t *ftt.Test) {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, projectReimportGroup))
					allowed, err := CanReimportConfigSet(ctx, projectConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeTrue)
				})
			})
			t.Run("Service", func(t *ftt.Test) {
				t.Run("Denied", func(t *ftt.Test) {
					allowed, err := CanReimportConfigSet(ctx, serviceConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeFalse)
				})
				t.Run("Allowed", func(t *ftt.Test) {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, serviceReimportGroup))
					allowed, err := CanReimportConfigSet(ctx, serviceConfigSet)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeTrue)
				})
			})
		})
	})
}
