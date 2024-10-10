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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	ftt.Run("Templates", t, func(t *ftt.Test) {
		ctx = auth.WithState(ctx, fakeAuthState())
		c := &router.Context{
			Request: (&http.Request{}).WithContext(ctx),
		}
		templateBundleMW := templates.WithTemplates(getTemplateBundle("test-version-1"))
		templateBundleMW(c, func(c *router.Context) {
			top, err := templates.Render(c.Request.Context(), "pages/index.html", nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(top), should.ContainSubstring("user@example.com"))
			assert.Loosely(t, string(top), should.ContainSubstring("test-version-1"))
		})
	})

	ftt.Run("InstallHandlers", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
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
		assert.Loosely(t, err, should.BeNil)
		bd, err = cl.WriteBlob(context.Background(), b)
		assert.Loosely(t, err, should.BeNil)
		rDict := fmt.Sprintf("/%s/blobs/%s/%d", testInstance, bd.Hash, bd.Size)

		// Unknown blob.
		rUnknown := fmt.Sprintf("/%s/blobs/12345/6", testInstance)

		// Invalid digest size.
		rInvalidDigest := fmt.Sprintf("/%s/blobs/12345/a", testInstance)

		t.Run("rootHanlder", func(t *ftt.Test) {
			resp, err := http.Get(srv.URL)
			assert.Loosely(t, err, should.BeNil)
			defer resp.Body.Close()

			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusOK))
			_, err = io.ReadAll(resp.Body)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("treeHandler", func(t *ftt.Test) {
			// Not found.
			resp, err := http.Get(srv.URL + rUnknown + "/tree")
			assert.Loosely(t, err, should.BeNil)
			resp.Body.Close()
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusNotFound))

			// Bad Request - Must be Directory.
			resp, err = http.Get(srv.URL + rSimple + "/tree")
			assert.Loosely(t, err, should.BeNil)
			resp.Body.Close()
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusBadRequest))

			// Bad Request - Digest size must be number.
			resp, err = http.Get(srv.URL + rInvalidDigest + "/tree")
			assert.Loosely(t, err, should.BeNil)
			resp.Body.Close()
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusBadRequest))

			// OK.
			resp, err = http.Get(srv.URL + rDict + "/tree")
			assert.Loosely(t, err, should.BeNil)
			resp.Body.Close()
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusOK))
		})

		t.Run("getHandler", func(t *ftt.Test) {
			// Not found.
			resp, err := http.Get(srv.URL + rUnknown)
			assert.Loosely(t, err, should.BeNil)
			resp.Body.Close()
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusNotFound))

			// Bad Request - Digest size must be number.
			resp, err = http.Get(srv.URL + rInvalidDigest)
			assert.Loosely(t, err, should.BeNil)
			resp.Body.Close()
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusBadRequest))

			// OK.
			resp, err = http.Get(srv.URL + rDict + "?filename=text.txt")
			assert.Loosely(t, err, should.BeNil)
			resp.Body.Close()
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusOK))
		})

		t.Run("checkPermission", func(t *ftt.Test) {
			resp, err := http.Get(
				srv.URL + "/projects/test-proj-no-perm/instances/default_instance/blobs/12345/6/tree")
			assert.Loosely(t, err, should.BeNil)
			defer resp.Body.Close()
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusForbidden))
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
