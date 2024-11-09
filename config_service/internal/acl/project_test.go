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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"
)

func TestProjectConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Project Config ACL Check", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		fakeAuthDB := authtest.NewFakeDB()
		const requester = identity.Identity("user:requester@example.com")
		const lProject = "example-project"
		realm := realms.Join(lProject, realms.RootRealm)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: requester,
			FakeDB:   fakeAuthDB,
		})
		const accessGroup = "access-group"
		const validationGroup = "validate-group"
		const reimportGroup = "reimport-group"
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ProjectAccessGroup:     accessGroup,
				ProjectValidationGroup: validationGroup,
				ProjectReimportGroup:   reimportGroup,
			},
		})

		t.Run("CanReadProject", func(t *ftt.Test) {
			t.Run("Access Denied", func(t *ftt.Test) {
				allowed, err := CanReadProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
			t.Run("allowed in Acl.cfg", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				allowed, err := CanReadProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})
			t.Run("allowed in realm", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				allowed, err := CanReadProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})
		})

		t.Run("CanReadProjects", func(t *ftt.Test) {
			const (
				project1 = "project1"
				project2 = "project2"
				project3 = "project3"
			)
			// can read project 1 and 3.
			fakeAuthDB.AddMocks(
				authtest.MockPermission(requester, realms.Join(project1, realms.RootRealm), ReadPermission),
				authtest.MockPermission(requester, realms.Join(project3, realms.RootRealm), ReadPermission),
			)
			result, err := CanReadProjects(ctx, []string{project1, project2, project3})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Resemble([]bool{true, false, true}))
		})

		t.Run("CanValidateProject", func(t *ftt.Test) {
			t.Run("Denied because of no read access", func(t *ftt.Test) {
				// This should allow validate
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, validationGroup))
				allowed, err := CanValidateProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
			t.Run("Denied even with read access", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				allowed, err := CanValidateProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
			t.Run("Allowed by Acl.cfg", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, validationGroup))
				allowed, err := CanValidateProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})
			t.Run("Allowed by Realm", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(
					authtest.MockPermission(requester, realm, ReadPermission),
					authtest.MockPermission(requester, realm, ValidatePermission),
				)
				allowed, err := CanValidateProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})
		})

		t.Run("CanReimportProject", func(t *ftt.Test) {
			t.Run("Denied because of no read access", func(t *ftt.Test) {
				// This should allow reimport
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, reimportGroup))
				allowed, err := CanReimportProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
			t.Run("Denied even with read access", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				allowed, err := CanReimportProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
			t.Run("Allowed by Acl.cfg", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, reimportGroup))
				allowed, err := CanReimportProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})
			t.Run("Allowed by Realm", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(
					authtest.MockPermission(requester, realm, ReadPermission),
					authtest.MockPermission(requester, realm, ReimportPermission),
				)
				allowed, err := CanReimportProject(ctx, lProject)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})
		})
	})
}
