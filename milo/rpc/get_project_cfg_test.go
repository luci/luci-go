// Copyright 2021 The LUCI Authors.
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

package rpc

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/milo/internal/projectconfig"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

func TestGetProjectCfg(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestGetProjectCfg`, t, func(t *ftt.Test) {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")
		datastore.GetTestable(c).Consistent(true)
		srv := &MiloInternalService{}

		mc := &projectconfigpb.MetadataConfig{
			TestMetadataProperties: []*projectconfigpb.DisplayRule{
				{
					Schema: "package.name",
					DisplayItems: []*projectconfigpb.DisplayItem{{
						DisplayName: "owner",
						Path:        "owner.email",
					}},
				},
			},
		}
		mcbytes, err := proto.Marshal(mc)
		assert.Loosely(t, err, should.BeNil)
		err = datastore.Put(c, &projectconfig.Project{
			ID:             "fake_project",
			ACL:            projectconfig.ACL{Identities: []identity.Identity{"user_with_access"}},
			LogoURL:        "https://logo.com",
			MetadataConfig: mcbytes,
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run(`reject users with no access`, func(t *ftt.Test) {
			c = auth.WithState(c, &authtest.FakeState{Identity: "user_without_access"})
			req := &milopb.GetProjectCfgRequest{
				Project: "fake_project",
			}

			_, err := srv.GetProjectCfg(c, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.PermissionDenied))
		})

		t.Run(`accept users access`, func(t *ftt.Test) {
			c = auth.WithState(c, &authtest.FakeState{Identity: "user_with_access"})
			req := &milopb.GetProjectCfgRequest{
				Project: "fake_project",
			}

			cfg, err := srv.GetProjectCfg(c, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.GetLogoUrl(), should.Equal("https://logo.com"))
			assert.Loosely(t, cfg.MetadataConfig, should.Resemble(mc))
		})

		t.Run(`reject invalid request`, func(t *ftt.Test) {
			c = auth.WithState(c, &authtest.FakeState{Identity: "user_with_access"})
			req := &milopb.GetProjectCfgRequest{}

			_, err := srv.GetProjectCfg(c, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.InvalidArgument))
		})
	})
}
