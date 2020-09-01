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
	"google.golang.org/api/idtoken"

	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/iap"
	"go.chromium.org/luci/server/router"
)

const testInstance = "projects/test-proj/instances/default_instance"

func TestHandlers(t *testing.T) {
	t.Parallel()

	Convey("InstallHandlers", t, func() {
		srv := setupServer(t)

		resp, err := GET(srv, "")
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

		resp, err := GET(srv, "/projects/test-proj/instances/default_instance/blobs/12345/6/tree")
		So(err, ShouldBeNil)
		defer resp.Body.Close()
		So(resp.StatusCode, ShouldEqual, http.StatusOK)
	})

	Convey("getHandler", t, func() {
		srv := setupServer(t)

		resp, err := GET(srv, "/projects/test-proj/instances/default_instance/blobs/12345/6")
		So(err, ShouldBeNil)
		defer resp.Body.Close()
		So(resp.StatusCode, ShouldEqual, http.StatusOK)
	})
}

// setupServer sets up a server.
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
	InstallHandlers(r, cc, fakeValidator)

	return httptest.NewServer(r)
}

// GET sends a GET request to the test server.
func GET(s *httptest.Server, url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", s.URL+url, nil)
	So(err, ShouldBeNil)
	req.Header.Add(iap.IapJWTAssertionHeader, "dummy iap header")
	return s.Client().Do(req)
}

func fakeValidator(
	ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
	return &idtoken.Payload{
		Issuer:   "",
		IssuedAt: 0,
		Subject:  "",
		Claims: map[string]interface{}{
			"email": "user@example.com",
		},
	}, nil
}
