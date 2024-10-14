// Copyright 2015 The LUCI Authors.
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

package deprecated

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/settings"
)

func TestFullFlow(t *testing.T) {
	t.Parallel()

	ftt.Run("with test context", t, func(c *ftt.Test) {
		ctx := context.Background()
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = authtest.MockAuthConfig(ctx)
		ctx = settings.Use(ctx, settings.New(&settings.MemoryStorage{}))
		ctx, _ = testclock.UseTime(ctx, time.Unix(1442540000, 0))
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		// Prepare the signing keys and the ID token.
		const signingKeyID = "signing-key"
		const clientID = "client_id"
		signer := signingtest.NewSigner(nil)
		idToken := idTokenForTest(ctx, &openid.IDToken{
			Iss:           "https://issuer.example.com",
			EmailVerified: true,
			Sub:           "user_id_sub",
			Email:         "user@example.com",
			Name:          "Some Dude",
			Picture:       "https://picture/url/s64/photo.jpg",
			Aud:           clientID,
			Iat:           clock.Now(ctx).Unix(),
			Exp:           clock.Now(ctx).Add(time.Hour).Unix(),
		}, signingKeyID, signer)
		jwks := jwksForTest(signingKeyID, &signer.KeyForTest().PublicKey)

		var ts *httptest.Server
		ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {

			case "/discovery":
				w.Write([]byte(fmt.Sprintf(`{
					"issuer": "https://issuer.example.com",
					"authorization_endpoint": "%s/authorization",
					"token_endpoint": "%s/token",
					"jwks_uri": "%s/jwks"
				}`, ts.URL, ts.URL, ts.URL)))

			case "/jwks":
				json.NewEncoder(w).Encode(jwks)

			case "/token":
				assert.Loosely(c, r.ParseForm(), should.BeNil)
				assert.Loosely(c, r.Form, should.Resemble(url.Values{
					"redirect_uri":  {"http://fake/redirect"},
					"client_id":     {"client_id"},
					"client_secret": {"client_secret"},
					"code":          {"omg_auth_code"},
					"grant_type":    {"authorization_code"},
				}))
				w.Write([]byte(fmt.Sprintf(`{"id_token": "%s"}`, idToken)))

			default:
				http.Error(w, "Not found", http.StatusNotFound)
			}
		}))
		defer ts.Close()

		cfg := Settings{
			DiscoveryURL: ts.URL + "/discovery",
			ClientID:     clientID,
			ClientSecret: "client_secret",
			RedirectURI:  "http://fake/redirect",
		}
		assert.Loosely(c, settings.Set(ctx, SettingsKey, &cfg), should.BeNil)

		method := CookieAuthMethod{
			SessionStore:        &MemorySessionStore{},
			Insecure:            true,
			IncompatibleCookies: []string{"wrong_cookie"},
		}

		c.Run("Full flow", func(c *ftt.Test) {
			assert.Loosely(c, method.Warmup(ctx), should.BeNil)

			// Generate login URL.
			loginURL, err := method.LoginURL(ctx, "/destination")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, loginURL, should.Equal("/auth/openid/login?r=%2Fdestination"))

			// "Visit" login URL.
			req, err := http.NewRequestWithContext(ctx, "GET", "http://fake"+loginURL, nil)
			assert.Loosely(c, err, should.BeNil)
			rec := httptest.NewRecorder()
			method.loginHandler(&router.Context{
				Writer:  rec,
				Request: req,
			})

			// It asks us to visit authorizarion endpoint.
			assert.Loosely(c, rec.Code, should.Equal(http.StatusFound))
			parsed, err := url.Parse(rec.Header().Get("Location"))
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, parsed.Host, should.Equal(ts.URL[len("http://"):]))
			assert.Loosely(c, parsed.Path, should.Equal("/authorization"))
			assert.Loosely(c, parsed.Query(), should.Resemble(url.Values{
				"client_id":     {"client_id"},
				"redirect_uri":  {"http://fake/redirect"},
				"response_type": {"code"},
				"scope":         {"openid email profile"},
				"prompt":        {"select_account"},
				"state": {
					"AXsiX2kiOiIxNDQyNTQwMDAwMDAwIiwiZGVzdF91cmwiOiIvZGVzdGluYXRpb24iLC" +
						"Job3N0X3VybCI6ImZha2UifUFtzG6wPbuvHG2mY_Wf6eQ_Eiu7n3_Tf6GmRcse1g" +
						"YE",
				},
			}))

			// Pretend we've done it. OpenID redirects user's browser to callback URI.
			// `callbackHandler` will call /token and /jwks fake endpoints exposed
			// by testserver.
			callbackParams := url.Values{}
			callbackParams.Set("code", "omg_auth_code")
			callbackParams.Set("state", parsed.Query().Get("state"))
			req, err = http.NewRequestWithContext(ctx, "GET", "http://fake/redirect?"+callbackParams.Encode(), nil)
			assert.Loosely(c, err, should.BeNil)
			rec = httptest.NewRecorder()
			method.callbackHandler(&router.Context{
				Writer:  rec,
				Request: req,
			})

			// We should be redirected to the login page, with session cookie set.
			expectedCookie := "oid_session=AXsiX2kiOiIxNDQyNTQwMDAwMDAwIiwic2lkIjoi" +
				"dXNlcl9pZF9zdWIvMSJ9PmRzaOv-mS0PMHkve897iiELNmpiLi_j3ICG1VKuNCs"
			assert.Loosely(c, rec.Code, should.Equal(http.StatusFound))
			assert.Loosely(c, rec.Header().Get("Location"), should.Equal("/destination"))
			assert.Loosely(c, rec.Header().Get("Set-Cookie"), should.Equal(
				expectedCookie+"; Path=/; Expires=Sun, 18 Oct 2015 01:18:20 GMT; Max-Age=2591100; HttpOnly"))

			// Use the cookie to authenticate some call.
			req, err = http.NewRequest("GET", "http://fake/something", nil)
			assert.Loosely(c, err, should.BeNil)
			req.Header.Add("Cookie", expectedCookie)
			user, session, err := method.Authenticate(ctx, auth.RequestMetadataForHTTP(req))
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, user, should.Resemble(&auth.User{
				Identity: "user:user@example.com",
				Email:    "user@example.com",
				Name:     "Some Dude",
				Picture:  "https://picture/url/s64/photo.jpg",
			}))
			assert.Loosely(c, session, should.BeNil)

			// Now generate URL to and visit logout page.
			logoutURL, err := method.LogoutURL(ctx, "/another_destination")
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, logoutURL, should.Equal("/auth/openid/logout?r=%2Fanother_destination"))
			req, err = http.NewRequestWithContext(ctx, "GET", "http://fake"+logoutURL, nil)
			assert.Loosely(c, err, should.BeNil)
			req.Header.Add("Cookie", expectedCookie)
			rec = httptest.NewRecorder()
			method.logoutHandler(&router.Context{
				Writer:  rec,
				Request: req,
			})

			// Should be redirected to destination with the cookie killed.
			assert.Loosely(c, rec.Code, should.Equal(http.StatusFound))
			assert.Loosely(c, rec.Header().Get("Location"), should.Equal("/another_destination"))
			assert.Loosely(c, rec.Header().Get("Set-Cookie"), should.Equal(
				"oid_session=deleted; Path=/; Expires=Thu, 01 Jan 1970 00:00:01 GMT; Max-Age=0"))
		})
	})
}

func TestCallbackHandleEdgeCases(t *testing.T) {
	ftt.Run("with test context", t, func(c *ftt.Test) {
		ctx := context.Background()
		ctx = settings.Use(ctx, settings.New(&settings.MemoryStorage{}))
		ctx, _ = testclock.UseTime(ctx, time.Unix(1442540000, 0))
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		method := CookieAuthMethod{SessionStore: &MemorySessionStore{}}

		call := func(query map[string]string) *httptest.ResponseRecorder {
			q := url.Values{}
			for k, v := range query {
				q.Add(k, v)
			}
			req, err := http.NewRequestWithContext(ctx, "GET", "/auth/openid/callback?"+q.Encode(), nil)
			assert.Loosely(c, err, should.BeNil)
			req.Host = "fake.com"
			rec := httptest.NewRecorder()
			method.callbackHandler(&router.Context{
				Writer:  rec,
				Request: req,
			})
			return rec
		}

		c.Run("handles 'error'", func(c *ftt.Test) {
			rec := call(map[string]string{"error": "Omg, error"})
			assert.Loosely(c, rec.Code, should.Equal(400))
			assert.Loosely(c, rec.Body.String(), should.Equal("OpenID login error: Omg, error\n"))
		})

		c.Run("handles no 'code'", func(c *ftt.Test) {
			rec := call(map[string]string{})
			assert.Loosely(c, rec.Code, should.Equal(400))
			assert.Loosely(c, rec.Body.String(), should.Equal("Missing 'code' parameter\n"))
		})

		c.Run("handles no 'state'", func(c *ftt.Test) {
			rec := call(map[string]string{"code": "123"})
			assert.Loosely(c, rec.Code, should.Equal(400))
			assert.Loosely(c, rec.Body.String(), should.Equal("Missing 'state' parameter\n"))
		})

		c.Run("handles bad 'state'", func(c *ftt.Test) {
			rec := call(map[string]string{"code": "123", "state": "garbage"})
			assert.Loosely(c, rec.Code, should.Equal(400))
			assert.Loosely(c, rec.Body.String(), should.Equal("Failed to validate 'state' token\n"))
		})

		c.Run("handles redirect to another host", func(c *ftt.Test) {
			state := map[string]string{
				"dest_url": "/",
				"host_url": "non-default.fake.com",
			}
			stateTok, err := openIDStateToken.Generate(ctx, nil, state, 0)
			assert.Loosely(c, err, should.BeNil)

			rec := call(map[string]string{"code": "123", "state": stateTok})
			assert.Loosely(c, rec.Code, should.Equal(302))
			assert.Loosely(c, rec.Header().Get("Location"), should.Equal(
				"https://non-default.fake.com/auth/openid/callback?"+
					"code=123&state=AXsiX2kiOiIxNDQyNTQwMDAwMDAwIiwiZGVzdF91cmwiOiIvIiw"+
					"iaG9zdF91cmwiOiJub24tZGVmYXVsdC5mYWtlLmNvbSJ92y0UJtCrN2qGYbcbCiZsV"+
					"9OdFEa3zAauzz4lmwPJLwI"))
		})
	})
}

func TestNotConfigured(t *testing.T) {
	ftt.Run("Returns ErrNotConfigured is on SessionStore", t, func(t *ftt.Test) {
		ctx := context.Background()
		method := CookieAuthMethod{}

		_, err := method.LoginURL(ctx, "/")
		assert.Loosely(t, err, should.Equal(ErrNotConfigured))

		_, err = method.LogoutURL(ctx, "/")
		assert.Loosely(t, err, should.Equal(ErrNotConfigured))

		_, _, err = method.Authenticate(ctx, authtest.NewFakeRequestMetadata())
		assert.Loosely(t, err, should.Equal(ErrNotConfigured))
	})
}

func TestNormalizeURL(t *testing.T) {
	ftt.Run("Normalizes good URLs", t, func(ctx *ftt.Test) {
		cases := []struct {
			in  string
			out string
		}{
			{"/", "/"},
			{"/?asd=def#blah", "/?asd=def#blah"},
			{"/abc/def", "/abc/def"},
			{"/blah//abc///def/", "/blah/abc/def/"},
			{"/blah/..//./abc/", "/abc/"},
			{"/abc/%2F/def", "/abc/def"},
		}
		for _, c := range cases {
			out, err := normalizeURL(c.in)
			if err != nil {
				ctx.Logf("Failed while checking %q\n", c.in)
				assert.Loosely(ctx, err, should.BeNil)
			}
			assert.Loosely(ctx, out, should.Equal(c.out))
		}
	})

	ftt.Run("Rejects bad URLs", t, func(ctx *ftt.Test) {
		cases := []string{
			"",
			"//",
			"///",
			"://",
			":",
			"http://another/abc/def",
			"abc/def",
			"//host.example.com",
		}
		for _, c := range cases {
			_, err := normalizeURL(c)
			if err == nil {
				ctx.Logf("Didn't fail while testing %q\n", c)
			}
			assert.Loosely(ctx, err, should.NotBeNil)
		}
	})
}

func TestBadDestinationURLs(t *testing.T) {
	ftt.Run("Rejects bad destination URLs", t, func(t *ftt.Test) {
		ctx := context.Background()
		method := CookieAuthMethod{SessionStore: &MemorySessionStore{}}

		_, err := method.LoginURL(ctx, "http://somesite")
		assert.Loosely(t, err, should.ErrLike("openid: dest URL in LoginURL or LogoutURL must be relative"))

		_, err = method.LogoutURL(ctx, "http://somesite")
		assert.Loosely(t, err, should.ErrLike("openid: dest URL in LoginURL or LogoutURL must be relative"))
	})
}
