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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServiceConfig(t *testing.T) {
	t.Parallel()

	Convey("Service Config ACL Check", t, func() {
		ctx := testutil.SetupContext()
		fakeAuthDB := authtest.NewFakeDB()
		const requester = identity.Identity("user:requester@example.com")
		const service = "my-service"
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: requester,
			FakeDB:   fakeAuthDB,
		})
		const accessGroup = "access-group"
		const validationGroup = "validate-group"
		const reimportGroup = "reimport-group"
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ServiceAccessGroup:     accessGroup,
				ServiceValidationGroup: validationGroup,
				ServiceReimportGroup:   reimportGroup,
			},
		})

		Convey("CanReadService", func() {
			Convey("Access Denied", func() {
				allowed, err := CanReadService(ctx, service)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("allowed in AccessGroup", func() {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				allowed, err := CanReadService(ctx, service)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})
		})

		Convey("CanReadServices", func() {
			services := []string{"service1", "service2"}
			result, err := CanReadServices(ctx, services)
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []bool{false, false})
			// allow access
			fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
			result, err = CanReadServices(ctx, services)
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []bool{true, true})
		})

		Convey("CanValidateService", func() {
			Convey("Denied because of no read access", func() {
				// This should allow validate
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, validationGroup))
				allowed, err := CanValidateService(ctx, service)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("Denied even with read access", func() {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				allowed, err := CanValidateService(ctx, service)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("Allowed by ValidationGroup", func() {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, validationGroup))
				allowed, err := CanValidateService(ctx, service)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})
		})

		Convey("CanReimportService", func() {
			Convey("Denied because of no read access", func() {
				// This should allow reimport
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, reimportGroup))
				allowed, err := CanReimportService(ctx, service)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("Denied even with read access", func() {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				allowed, err := CanReimportService(ctx, service)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeFalse)
			})
			Convey("Allowed by ReimportGroup", func() {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, reimportGroup))
				allowed, err := CanReimportService(ctx, service)
				So(err, ShouldBeNil)
				So(allowed, ShouldBeTrue)
			})
		})
	})
}
