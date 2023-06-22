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
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProjectConfig(t *testing.T) {
	t.Parallel()

	Convey("Project Config ACL Check", t, func() {
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
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ProjectAccessGroup:     accessGroup,
				ProjectValidationGroup: validationGroup,
				ProjectReimportGroup:   reimportGroup,
			},
		})

		Convey("CanReadProject", func() {
			Convey("Access Denied", func() {
				allowed, err := CanReadProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("allowed in Acl.cfg", func() {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				allowed, err := CanReadProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})
			Convey("allowed in realm", func() {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				allowed, err := CanReadProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})
		})

		Convey("CanReadProjects", func() {
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
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []bool{true, false, true})
		})

		Convey("CanValidateProject", func() {
			Convey("Denied because of no read access", func() {
				// This should allow validate
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, validationGroup))
				allowed, err := CanValidateProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("Denied even with read access", func() {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				allowed, err := CanValidateProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("Allowed by Acl.cfg", func() {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, validationGroup))
				allowed, err := CanValidateProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})
			Convey("Allowed by Realm", func() {
				fakeAuthDB.AddMocks(
					authtest.MockPermission(requester, realm, ReadPermission),
					authtest.MockPermission(requester, realm, ValidatePermission),
				)
				allowed, err := CanValidateProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})
		})

		Convey("CanReimportProject", func() {
			Convey("Denied because of no read access", func() {
				// This should allow reimport
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, reimportGroup))
				allowed, err := CanReimportProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("Denied even with read access", func() {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				allowed, err := CanReimportProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("Allowed by Acl.cfg", func() {
				fakeAuthDB.AddMocks(authtest.MockPermission(requester, realm, ReadPermission))
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, reimportGroup))
				allowed, err := CanReimportProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})
			Convey("Allowed by Realm", func() {
				fakeAuthDB.AddMocks(
					authtest.MockPermission(requester, realm, ReadPermission),
					authtest.MockPermission(requester, realm, ReimportPermission),
				)
				allowed, err := CanReimportProject(ctx, lProject)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})
		})
	})
}
