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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/grpc/codes"
)

func TestGetProjectCfg(t *testing.T) {
	t.Parallel()
	Convey(`TestGetProjectCfg`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")
		datastore.GetTestable(c).Consistent(true)
		srv := &MiloInternalService{}

		err := datastore.Put(c, &common.Project{
			ID:      "fake_project",
			ACL:     common.ACL{Identities: []identity.Identity{"user_with_access"}},
			LogoURL: "https://logo.com",
		})
		So(err, ShouldBeNil)

		Convey(`reject users with no access`, func() {
			c = auth.WithState(c, &authtest.FakeState{Identity: "user_without_access"})
			req := &milopb.GetProjectCfgRequest{
				Project: "fake_project",
			}

			_, err := srv.GetProjectCfg(c, req)
			So(err, ShouldNotBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey(`accept users access`, func() {
			c = auth.WithState(c, &authtest.FakeState{Identity: "user_with_access"})
			req := &milopb.GetProjectCfgRequest{
				Project: "fake_project",
			}

			cfg, err := srv.GetProjectCfg(c, req)
			So(err, ShouldBeNil)
			So(cfg.GetLogoUrl(), ShouldEqual, "https://logo.com")
		})

		Convey(`reject invalid request`, func() {
			c = auth.WithState(c, &authtest.FakeState{Identity: "user_with_access"})
			req := &milopb.GetProjectCfgRequest{}

			_, err := srv.GetProjectCfg(c, req)
			So(err, ShouldNotBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
		})
	})
}
