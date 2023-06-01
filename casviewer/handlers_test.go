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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

const testInstance = "projects/test-proj/instances/default_instance"

func TestHandlers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Basic template rendering tests.
	Convey("Templates", t, func() {
		ctx = auth.WithState(ctx, fakeAuthState())
		c := &router.Context{
			Request: (&http.Request{}).WithContext(ctx),
		}
		templateBundleMW := templates.WithTemplates(getTemplateBundle("test-version-1"))
		templateBundleMW(c, func(c *router.Context) {
			top, err := templates.Render(c.Request.Context(), "pages/index.html", nil)
			So(err, ShouldBeNil)
			So(string(top), ShouldContainSubstring, "user@example.com")
			So(string(top), ShouldContainSubstring, "test-version-1")
		})
	})

	Convey("InstallHandlers", t, func() {
		// Install handlers with fake auth state.
		r := router.New()
		r.Use(router.NewMiddlewareChain(func(c *router.Context, next router.Handler) {
			c.Request = c.Request.WithContext(auth.WithState(c.Request.Context(), fakeAuthState()))
			next(c)
		}))

		// Inject fake CAS client to cache.
		cl := fakeClient(ctx, t)
		cc := NewClientCache(ctx)
		t.Cleanup(cc.Clear)
		cc.clients[testInstance] = cl

		InstallHandlers(r, cc, "test-version-1")

		srv := httptest.NewServer(r)
		t.Cleanup(srv.Close)

		// Simple blob.
		bd, err := cl.WriteBlob(ctx, []byte{1})
		So(err, ShouldBeNil)
		rSimple := fmt.Sprintf("/%s/blobs/%s/%d", testInstance, bd.Hash, bd.Size)

		// Directory.
		d := &repb.Directory{
			Files: []*repb.FileNode{
				{
					Name:   "foo",
					Digest: digest.NewFromBlob([]byte{1}).ToProto(),
				},
			},
		}
		b, err := proto.Marshal(d)
		So(err, ShouldBeNil)
		bd, err = cl.WriteBlob(context.Background(), b)
		So(err, ShouldBeNil)
		rDict := fmt.Sprintf("/%s/blobs/%s/%d", testInstance, bd.Hash, bd.Size)

		// Unknown blob.
		rUnknown := fmt.Sprintf("/%s/blobs/12345/6", testInstance)

		// Invalid digest size.
		rInvalidDigest := fmt.Sprintf("/%s/blobs/12345/a", testInstance)

		Convey("rootHanlder", func() {
			resp, err := http.Get(srv.URL)
			So(err, ShouldBeNil)
			defer resp.Body.Close()

			So(resp.StatusCode, ShouldEqual, http.StatusOK)
			_, err = io.ReadAll(resp.Body)
			So(err, ShouldBeNil)
		})

		Convey("treeHandler", func() {
			// Not found.
			resp, err := http.Get(srv.URL + rUnknown + "/tree")
			So(err, ShouldBeNil)
			resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, http.StatusNotFound)

			// Bad Request - Must be Directory.
			resp, err = http.Get(srv.URL + rSimple + "/tree")
			So(err, ShouldBeNil)
			resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, http.StatusBadRequest)

			// Bad Request - Digest size must be number.
			resp, err = http.Get(srv.URL + rInvalidDigest + "/tree")
			So(err, ShouldBeNil)
			resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, http.StatusBadRequest)

			// OK.
			resp, err = http.Get(srv.URL + rDict + "/tree")
			So(err, ShouldBeNil)
			resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
		})

		Convey("getHandler", func() {
			// Not found.
			resp, err := http.Get(srv.URL + rUnknown)
			So(err, ShouldBeNil)
			resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, http.StatusNotFound)

			// Bad Request - Digest size must be number.
			resp, err = http.Get(srv.URL + rInvalidDigest)
			So(err, ShouldBeNil)
			resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, http.StatusBadRequest)

			// OK.
			resp, err = http.Get(srv.URL + rDict + "?filename=text.txt")
			So(err, ShouldBeNil)
			resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
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

// fakeAuthState returns fake state that has identity and realm permission.
func fakeAuthState() *authtest.FakeState {
	return &authtest.FakeState{
		Identity: "user:user@example.com",
		IdentityPermissions: []authtest.RealmPermission{
			{
				Realm:      "@internal:test-proj/cas-read-only",
				Permission: permMintToken,
			},
		},
	}
}
