// Copyright 2016 The LUCI Authors.
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

package handlers

import (
	"bytes"
	"html/template"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/milo/internal/git"
	"go.chromium.org/luci/milo/internal/projectconfig"
)

func TestFuncs(t *testing.T) {
	t.Parallel()

	ftt.Run("Middleware Tests", t, func(t *ftt.Test) {
		t.Run("Format Commit Description", func(t *ftt.Test) {
			t.Run("linkify https://", func(t *ftt.Test) {
				assert.Loosely(t, formatCommitDesc("https://foo.com"),
					should.Equal(
						template.HTML("<a href=\"https://foo.com\">https://foo.com</a>")))
				t.Run("but not http://", func(t *ftt.Test) {
					assert.Loosely(t, formatCommitDesc("http://foo.com"), should.Equal(template.HTML("http://foo.com")))
				})
			})
			t.Run("linkify b/ and crbug/", func(t *ftt.Test) {
				assert.Loosely(t, formatCommitDesc("blah blah b/123456 blah"), should.Equal(template.HTML("blah blah <a href=\"http://b/123456\">b/123456</a> blah")))
				assert.Loosely(t, formatCommitDesc("crbug:foo/123456"), should.Equal(template.HTML("<a href=\"https://crbug.com/foo/123456\">crbug:foo/123456</a>")))
			})
			t.Run("linkify Bug: lines", func(t *ftt.Test) {
				assert.Loosely(t, formatCommitDesc("\nBug: 12345\n"), should.Equal(template.HTML("\nBug: <a href=\"https://crbug.com/12345\">12345</a>\n")))
				assert.Loosely(t,
					formatCommitDesc(" > > BugS=  12345, butter:12345"),
					should.Equal(
						template.HTML(" &gt; &gt; BugS=  <a href=\"https://crbug.com/12345\">12345</a>, "+
							"<a href=\"https://crbug.com/butter/12345\">butter:12345</a>")))
			})
			t.Run("linkify rules should not collide", func(t *ftt.Test) {
				assert.Loosely(t,
					formatCommitDesc("I \"fixed\" https://crbug.com/123456 <today>"),
					should.Equal(
						template.HTML("I &#34;fixed&#34; <a href=\"https://crbug.com/123456\">https://crbug.com/123456</a> &lt;today&gt;")))
				assert.Loosely(t,
					formatCommitDesc("Bug: 12, crbug/34, https://crbug.com/56, 78"),
					should.Equal(
						template.HTML("Bug: <a href=\"https://crbug.com/12\">12</a>, <a href=\"https://crbug.com/34\">crbug/34</a>, <a href=\"https://crbug.com/56\">https://crbug.com/56</a>, <a href=\"https://crbug.com/78\">78</a>")))
			})
			t.Run("linkify rules interact correctly with escaping", func(t *ftt.Test) {
				assert.Loosely(t,
					formatCommitDesc("\"https://example.com\""),
					should.Equal(
						template.HTML("&#34;<a href=\"https://example.com\">https://example.com</a>&#34;")))
				assert.Loosely(t,
					formatCommitDesc("Bug: <not a bug number, sorry>"),
					should.Equal(
						template.HTML("Bug: &lt;not a bug number, sorry&gt;")))
				// This is not remotely valid of a URL, but exists to test that
				// the linking template correctly escapes the URL, both as an
				// attribute and as a value.
				assert.Loosely(t,
					formatCommitDesc("https://foo&bar<baz\"aaa>bbb"),
					should.Equal(
						template.HTML("<a href=\"https://foo&amp;bar%3cbaz%22aaa%3ebbb\">https://foo&amp;bar&lt;baz&#34;aaa&gt;bbb</a>")))
			})

			t.Run("trimLongString", func(t *ftt.Test) {
				t.Run("short", func(t *ftt.Test) {
					assert.Loosely(t, trimLongString(4, "😀😀😀😀"), should.Equal("😀😀😀😀"))
				})
				t.Run("long", func(t *ftt.Test) {
					assert.Loosely(t, trimLongString(4, "😀😀😀😀😀"), should.Equal("😀😀😀…"))
				})
			})
		})

		t.Run("Redirect unauthorized users to login page for projects with access restrictions", func(t *ftt.Test) {
			projectACLMiddleware := buildProjectACLMiddleware(false)
			r := httptest.NewRecorder()
			c := gaetesting.TestingContextWithAppID("luci-milo-dev")

			// Fake user to be anonymous.
			c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

			// Create fake internal project named "secret".
			c = cfgclient.Use(c, memory.New(map[config.Set]memory.Files{
				"projects/secret": {
					"project.cfg": "name: \"secret\"\naccess: \"group:googlers\"",
				},
			}))
			assert.Loosely(t, projectconfig.UpdateProjects(c), should.BeNil)

			ctx := &router.Context{
				Writer:  r,
				Request: httptest.NewRequest("GET", "/p/secret", bytes.NewReader(nil)).WithContext(c),
				Params:  httprouter.Params{{Key: "project", Value: "secret"}},
			}
			projectACLMiddleware(ctx, nil)
			project, ok := git.ProjectFromContext(ctx.Request.Context())
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, project, should.BeEmpty)
			assert.Loosely(t, r.Code, should.Equal(302))
			assert.Loosely(t, r.Result().Header["Location"], should.Resemble([]string{"http://fake.example.com/login?dest=%2Fp%2Fsecret"}))
		})

		t.Run("Install git project to context when the user has access to the project", func(t *ftt.Test) {
			optionalProjectACLMiddleware := buildProjectACLMiddleware(true)
			r := httptest.NewRecorder()
			c := gaetesting.TestingContextWithAppID("luci-milo-dev")

			// Fake user to be anonymous.
			c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity, IdentityGroups: []string{"all"}})

			// Create fake public project named "public".
			c = cfgclient.Use(c, memory.New(map[config.Set]memory.Files{
				"projects/public": {
					"project.cfg": "name: \"public\"\naccess: \"group:all\"",
				},
			}))
			assert.Loosely(t, projectconfig.UpdateProjects(c), should.BeNil)

			ctx := &router.Context{
				Writer:  r,
				Request: httptest.NewRequest("GET", "/p/public", bytes.NewReader(nil)).WithContext(c),
				Params:  httprouter.Params{{Key: "project", Value: "public"}},
			}
			nextCalled := false
			next := func(*router.Context) {
				nextCalled = true
			}
			optionalProjectACLMiddleware(ctx, next)
			project, ok := git.ProjectFromContext(ctx.Request.Context())
			assert.Loosely(t, project, should.Equal("public"))
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, nextCalled, should.BeTrue)
			assert.Loosely(t, r.Code, should.Equal(200))
		})

		t.Run("Don't install git project to context when the user doesn't have access to the project", func(t *ftt.Test) {
			optionalProjectACLMiddleware := buildProjectACLMiddleware(true)
			r := httptest.NewRecorder()
			c := gaetesting.TestingContextWithAppID("luci-milo-dev")

			// Fake user to be anonymous.
			c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

			// Create fake internal project named "secret".
			c = cfgclient.Use(c, memory.New(map[config.Set]memory.Files{
				"projects/secret": {
					"project.cfg": "name: \"secret\"\naccess: \"group:googlers\"",
				},
			}))
			assert.Loosely(t, projectconfig.UpdateProjects(c), should.BeNil)

			ctx := &router.Context{
				Writer:  r,
				Request: httptest.NewRequest("GET", "/p/secret", bytes.NewReader(nil)).WithContext(c),
				Params:  httprouter.Params{{Key: "project", Value: "secret"}},
			}
			nextCalled := false
			next := func(*router.Context) {
				nextCalled = true
			}
			optionalProjectACLMiddleware(ctx, next)
			project, ok := git.ProjectFromContext(ctx.Request.Context())
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, project, should.BeEmpty)
			assert.Loosely(t, nextCalled, should.BeTrue)
			assert.Loosely(t, r.Code, should.Equal(200))
		})

		t.Run("Convert LogDog URLs", func(t *ftt.Test) {
			assert.Loosely(t,
				logdogLink(buildbucketpb.Log{Name: "foo", Url: "logdog://www.example.com:1234/foo/bar/+/baz"}, true),
				should.Equal(
					template.HTML(`<a href="https://www.example.com:1234/logs/foo/bar/&#43;/baz?format=raw" aria-label="raw log foo">raw</a>`)))
			assert.Loosely(t,
				logdogLink(buildbucketpb.Log{Name: "foo", Url: "%zzzzz"}, true),
				should.Equal(
					template.HTML(`<a href="#invalid-logdog-link" aria-label="raw log foo">raw</a>`)))
			assert.Loosely(t,
				logdogLink(buildbucketpb.Log{Name: "foo", Url: "logdog://logs.chromium.org/foo/bar/+/baz"}, false),
				should.Equal(
					template.HTML(`<a href="https://logs.chromium.org/logs/foo/bar/&#43;/baz" aria-label="raw log foo">foo</a>`)))
		})
	})
}
