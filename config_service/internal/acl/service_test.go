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
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"
)

func TestServiceConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Service Config ACL Check", t, func(t *ftt.Test) {
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
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ServiceAccessGroup:     accessGroup,
				ServiceValidationGroup: validationGroup,
				ServiceReimportGroup:   reimportGroup,
			},
		})

		t.Run("CanReadService", func(t *ftt.Test) {
			t.Run("Access Denied", func(t *ftt.Test) {
				allowed, err := CanReadService(ctx, service)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
				t.Run("Not even with consulting service config", func(t *ftt.Test) {
					srv := &model.Service{
						Name: service,
						Info: &cfgcommonpb.Service{
							Id:     service,
							Access: []string{"admin@example.com"},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, srv), should.BeNil)
					allowed, err := CanReadService(ctx, service)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeFalse)
				})
			})
			t.Run("Allowed in AccessGroup", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				allowed, err := CanReadService(ctx, service)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})
			t.Run("Allowed in service config", func(t *ftt.Test) {
				t.Run("By group", func(t *ftt.Test) {
					srv := &model.Service{
						Name: service,
						Info: &cfgcommonpb.Service{
							Id:     service,
							Access: []string{"group:some-access-group"},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, srv), should.BeNil)
					fakeAuthDB.AddMocks(authtest.MockMembership(requester, "some-access-group"))
					allowed, err := CanReadService(ctx, service)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, allowed, should.BeTrue)
				})
				t.Run("By explicit identity", func(t *ftt.Test) {
					for _, prefix := range []string{"", "user:"} {
						t.Run(fmt.Sprintf("With prefix %q", prefix), func(t *ftt.Test) {
							srv := &model.Service{
								Name: service,
								Info: &cfgcommonpb.Service{
									Id:     service,
									Access: []string{prefix + requester.Email()},
								},
							}
							assert.Loosely(t, datastore.Put(ctx, srv), should.BeNil)
							allowed, err := CanReadService(ctx, service)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, allowed, should.BeTrue)
						})
					}
				})
			})
		})

		t.Run("CanReadServices", func(t *ftt.Test) {
			services := []string{"service1", "service2"}
			result, err := CanReadServices(ctx, services)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Match([]bool{false, false}))
			// allow access
			fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
			result, err = CanReadServices(ctx, services)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Match([]bool{true, true}))
		})

		t.Run("CanValidateService", func(t *ftt.Test) {
			t.Run("Denied because of no read access", func(t *ftt.Test) {
				// This should allow validate
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, validationGroup))
				allowed, err := CanValidateService(ctx, service)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
			t.Run("Denied even with read access", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				allowed, err := CanValidateService(ctx, service)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
			t.Run("Allowed by ValidationGroup", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, validationGroup))
				allowed, err := CanValidateService(ctx, service)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})
		})

		t.Run("CanReimportService", func(t *ftt.Test) {
			t.Run("Denied because of no read access", func(t *ftt.Test) {
				// This should allow reimport
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, reimportGroup))
				allowed, err := CanReimportService(ctx, service)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
			t.Run("Denied even with read access", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				allowed, err := CanReimportService(ctx, service)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeFalse)
			})
			t.Run("Allowed by ReimportGroup", func(t *ftt.Test) {
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, accessGroup))
				fakeAuthDB.AddMocks(authtest.MockMembership(requester, reimportGroup))
				allowed, err := CanReimportService(ctx, service)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, allowed, should.BeTrue)
			})
		})
	})
}
