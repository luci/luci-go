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

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
)

func TestHandlers(t *testing.T) {
	t.Parallel()

	Convey("InstallHandlers", t, func() {
		srv := setupServer(t)

		resp, err := http.Get(srv.URL)
		So(err, ShouldBeNil)
		defer resp.Body.Close()

		So(resp.StatusCode, ShouldEqual, http.StatusOK)
		// Body should contain user email address.
		body, err := ioutil.ReadAll(resp.Body)
		So(err, ShouldBeNil)
		So(string(body), ShouldContainSubstring, "user@example.com")
	})

	Convey("treeHandler", t, func() {
		srv := setupServer(t)

		resp, err := http.Get(
			srv.URL + "/projects/test-proj/instances/default_instance/blobs/12345/6/tree")
		So(err, ShouldBeNil)
		defer resp.Body.Close()
		So(resp.StatusCode, ShouldEqual, http.StatusOK)
	})

	Convey("getHandler", t, func() {
		srv := setupServer(t)

		resp, err := http.Get(
			srv.URL + "/projects/test-proj/instances/default_instance/blobs/12345/6")
		So(err, ShouldBeNil)
		defer resp.Body.Close()
		So(resp.StatusCode, ShouldEqual, http.StatusOK)
	})
}

// setupServer sets up a server with fake auth settings.
func setupServer(t *testing.T) *httptest.Server {
	r := router.New()
	r.Use(router.NewMiddlewareChain(func(c *router.Context, next router.Handler) {
		c.Context = authtest.MockAuthConfig(c.Context)
		db := authtest.NewFakeDB()
		c.Context = db.Use(c.Context)
		next(c)
	}))

	cc := NewClientCache(context.Background())
	t.Cleanup(cc.Clear)
	InstallHandlers(r, cc, fakeAuthMiddleware())

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return srv
}

func fakeAuthMiddleware() router.Middleware {
	authMethods := []auth.Method{
		&authtest.FakeAuth{User: &auth.User{
			Identity: "user:user@example.com",
			Email:    "user@example.com",
		}},
	}
	a := &auth.Authenticator{Methods: authMethods}
	return a.GetMiddleware()

}
