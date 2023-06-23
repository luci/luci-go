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
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfigSet(t *testing.T) {
	t.Parallel()

	Convey("Config Set ACL Check", t, func() {
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
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ProjectAccessGroup:     projectAccessGroup,
				ProjectValidationGroup: projectValidationGroup,
				ProjectReimportGroup:   projectReimportGroup,
				ServiceAccessGroup:     serviceAccessGroup,
				ServiceValidationGroup: serviceValidationGroup,
				ServiceReimportGroup:   serviceReimportGroup,
			},
		})
		Convey("CanReadConfigSet", func() {
			Convey("Project", func() {
				Convey("Denied", func() {
					allowed, err := CanReadConfigSet(ctx, projectConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeFalse)
				})
				Convey("Allowed", func() {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, projectAccessGroup))
					allowed, err := CanReadConfigSet(ctx, projectConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeTrue)
				})
			})
			Convey("Service", func() {
				Convey("Denied", func() {
					allowed, err := CanReadConfigSet(ctx, serviceConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeFalse)
				})
				Convey("Allowed", func() {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, serviceAccessGroup))
					allowed, err := CanReadConfigSet(ctx, serviceConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeTrue)
				})
			})
		})

		Convey("CanReadConfigSets", func() {
			fakeAuthDB.AddMocks(
				authtest.MockPermission(requester, realms.Join(projectConfigSet.Project(), realms.RootRealm), ReadPermission),
				authtest.MockMembership(requester, serviceAccessGroup),
			)
			result, err := CanReadConfigSets(ctx, []config.Set{projectConfigSet, serviceConfigSet, config.MustServiceSet("another-service"), config.MustProjectSet("another-project")})
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []bool{true, true, true, false})
		})

		Convey("CanValidateConfigSet", func() {
			fakeAuthDB.AddMocks( // allow read access
				authtest.MockMembership(requester, projectAccessGroup),
				authtest.MockMembership(requester, serviceAccessGroup),
			)
			Convey("Project", func() {
				Convey("Denied", func() {
					allowed, err := CanValidateConfigSet(ctx, projectConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeFalse)
				})
				Convey("Allowed", func() {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, projectValidationGroup))
					allowed, err := CanValidateConfigSet(ctx, projectConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeTrue)
				})
			})
			Convey("Service", func() {
				Convey("Denied", func() {
					allowed, err := CanValidateConfigSet(ctx, serviceConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeFalse)
				})
				Convey("Allowed", func() {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, serviceValidationGroup))
					allowed, err := CanValidateConfigSet(ctx, serviceConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeTrue)
				})
			})
		})

		Convey("CanReimportProject", func() {
			fakeAuthDB.AddMocks( // allow read access
				authtest.MockMembership(requester, projectAccessGroup),
				authtest.MockMembership(requester, serviceAccessGroup),
			)
			Convey("Project", func() {
				Convey("Denied", func() {
					allowed, err := CanReimportConfigSet(ctx, projectConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeFalse)
				})
				Convey("Allowed", func() {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, projectReimportGroup))
					allowed, err := CanReimportConfigSet(ctx, projectConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeTrue)
				})
			})
			Convey("Service", func() {
				Convey("Denied", func() {
					allowed, err := CanReimportConfigSet(ctx, serviceConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeFalse)
				})
				Convey("Allowed", func() {
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, serviceReimportGroup))
					allowed, err := CanReimportConfigSet(ctx, serviceConfigSet)
					So(err, ShouldBeNil)
					So(allowed, ShouldBeTrue)
				})
			})
		})
	})
}
