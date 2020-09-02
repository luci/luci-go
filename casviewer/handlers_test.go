// Copyright 2020 The LUCI Authors.
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

package casviewer

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/router"
)

const testInstance = "projects/test-proj/instances/default_instance"

func TestHandlers(t *testing.T) {
	t.Parallel()

	Convey("InstallHandlers", t, func() {
		// Install handlers with fake auth settings and CAS server/client.
		r := router.New()

		// fake auth config and DB.
		r.Use(router.NewMiddlewareChain(func(c *router.Context, next router.Handler) {
			fakeAuthState := &authtest.FakeState{
				Identity: "user:user@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{
						Realm:      "@internal:test-proj/cas-read-only",
						Permission: realms.RegisterPermission("luci.serviceAccounts.mintToken"),
					},
				},
			}
			c.Context = auth.WithState(c.Context, fakeAuthState)
			c.Context = auth.ModifyConfig(c.Context, func(cfg auth.Config) auth.Config {
				cfg.DBProvider = func(context.Context) (authdb.DB, error) {
					return fakeAuthState.DB(), nil
				}
				return cfg
			})
			next(c)
		}))

		// fake CAS server.
		casSrv, err := fakes.NewServer(t)
		So(err, ShouldBeNil)

		// fake CAS client.
		cli, err := casSrv.NewTestClient(context.Background())
		t.Cleanup(casSrv.Clear)
		So(err, ShouldBeNil)

		// Inject fake CAS client to cache.
		cc := NewClientCache(context.Background())
		t.Cleanup(cc.Clear)
		cc.clients[testInstance] = cli

		InstallHandlers(r, cc)

		srv := httptest.NewServer(r)
		t.Cleanup(srv.Close)

		Convey("rootHanlder", func() {
			resp, err := http.Get(srv.URL)
			So(err, ShouldBeNil)
			defer resp.Body.Close()

			So(resp.StatusCode, ShouldEqual, http.StatusOK)
			// Body should contain user email address.
			body, err := ioutil.ReadAll(resp.Body)
			So(err, ShouldBeNil)
			So(string(body), ShouldContainSubstring, "user@example.com")
		})

		Convey("treeHandler", func() {
			resp, err := http.Get(
				srv.URL + "/projects/test-proj/instances/default_instance/blobs/12345/6/tree")
			So(err, ShouldBeNil)
			defer resp.Body.Close()
		})

		Convey("getHandler", func() {
			resp, err := http.Get(
				srv.URL + "/projects/test-proj/instances/default_instance/blobs/12345/6")
			So(err, ShouldBeNil)
			defer resp.Body.Close()
		})

		Convey("checkPermission", func() {
			resp, err := http.Get(
				srv.URL + "/projects/test-proj-no-perm/instances/default_instance/blobs/12345/6/tree")
			So(err, ShouldBeNil)
			defer resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, http.StatusForbidden)
		})
	})
}
