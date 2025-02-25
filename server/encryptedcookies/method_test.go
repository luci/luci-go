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

package encryptedcookies

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"
	"github.com/google/tink/go/tink"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/encryptedcookies/internal"
	"go.chromium.org/luci/server/encryptedcookies/session/datastore"
	"go.chromium.org/luci/server/router"
)

func TestMethod(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		// Instantiate ID provider with mocked time only. It doesn't need other
		// context features.
		ctx := context.Background()
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		provider := openIDProviderFake{
			ExpectedClientID:     "client_id",
			ExpectedClientSecret: "client_secret",
			ExpectedRedirectURI:  "https://primary.example.com/auth/openid/callback",
			UserSub:              "user-sub",
			UserEmail:            "someone@example.com",
			UserName:             "Someone Example",
			UserPicture:          "https://example.com/picture",
			RefreshToken:         "good_refresh_token",
		}
		provider.Init(ctx, t)
		defer provider.Close()

		// Primary encryption keys used to encrypt cookies.
		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		assert.Loosely(t, err, should.BeNil)
		ae, err := aead.New(kh)
		assert.Loosely(t, err, should.BeNil)

		// Install the rest of the context stuff used by AuthMethod.
		ctx = memory.Use(ctx)
		ctx = gologger.StdConfig.Use(ctx)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = authtest.MockAuthConfig(ctx)

		sessionStore := &datastore.Store{}

		makeMethod := func(requiredScopes, optionalScopes []string) *AuthMethod {
			return &AuthMethod{
				OpenIDConfig: func(context.Context) (*OpenIDConfig, error) {
					return &OpenIDConfig{
						DiscoveryURL: provider.DiscoveryURL(),
						ClientID:     provider.ExpectedClientID,
						ClientSecret: provider.ExpectedClientSecret,
						RedirectURI:  provider.ExpectedRedirectURI,
					}, nil
				},
				AEADProvider:        func(context.Context) tink.AEAD { return ae },
				Sessions:            sessionStore,
				RequiredScopes:      requiredScopes,
				OptionalScopes:      optionalScopes,
				ExposeStateEndpoint: true,
			}
		}

		methodV1 := makeMethod([]string{"scope1"}, []string{"scope2"})

		call := func(h router.Handler, host string, url *url.URL, header http.Header) *http.Response {
			rw := httptest.NewRecorder()
			h(&router.Context{
				Request: (&http.Request{
					URL:    url,
					Host:   host,
					Header: header,
				}).WithContext(ctx),
				Writer: rw,
			})
			return rw.Result()
		}

		performLogin := func(method *AuthMethod) (callbackRawQuery string) {
			assert.Loosely(t, method.Warmup(ctx), should.BeNil)

			loginURL, err := method.LoginURL(ctx, "/some/dest")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, loginURL, should.Equal("/auth/openid/login?r=%2Fsome%2Fdest"))
			parsed, _ := url.Parse(loginURL)

			// Hitting the generated login URL generates a redirect to the provider.
			resp := call(method.loginHandler, "dest.example.com", parsed, nil)
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusFound))
			authURL, _ := url.Parse(resp.Header.Get("Location"))
			assert.Loosely(t, authURL.Host, should.Equal(provider.Host()))
			assert.Loosely(t, authURL.Path, should.Equal("/authorization"))

			// After the user logs in, the provider generates a redirect to the
			// callback URI with some query parameters.
			return provider.CallbackRawQuery(authURL.Query())
		}

		performCallback := func(method *AuthMethod, callbackRawQuery string) (cookie *http.Cookie, deleted []*http.Cookie) {
			// Provider calls us back on our primary host (not "dest.example.com").
			resp := call(method.callbackHandler, "primary.example.com", &url.URL{
				RawQuery: callbackRawQuery,
			}, nil)

			// We got a redirect to "dest.example.com".
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusFound))
			assert.Loosely(t, resp.Header.Get("Location"), should.Equal("https://dest.example.com?"+callbackRawQuery))

			// Now hitting the same callback on "dest.example.com".
			resp = call(method.callbackHandler, "dest.example.com", &url.URL{
				RawQuery: callbackRawQuery,
			}, nil)

			// Got a redirect to the final destination URL.
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusFound))
			assert.Loosely(t, resp.Header.Get("Location"), should.Equal("/some/dest"))

			// And we've got some session cookie!
			for _, c := range resp.Cookies() {
				if c.MaxAge == -1 {
					deleted = append(deleted, c)
				} else {
					// Should have at most one new cookie.
					assert.Loosely(t, cookie, should.BeNil)
					cookie = c
				}
			}
			assert.Loosely(t, cookie, should.NotBeNil)
			return
		}

		phonyRequest := func(cookie *http.Cookie) auth.RequestMetadata {
			req, _ := http.NewRequest("GET", "https://dest.example.com/phony", nil)
			if cookie != nil {
				req.Header.Add("Cookie", fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
			}
			return auth.RequestMetadataForHTTP(req)
		}

		t.Run("Full flow", func(t *ftt.Test) {
			callbackRawQueryV1 := performLogin(methodV1)
			cookieV1, deleted := performCallback(methodV1, callbackRawQueryV1)

			// Set the cookie on the correct path.
			assert.Loosely(t, cookieV1.Path, should.Equal(internal.UnlimitedCookiePath))

			// Removed a potentially stale cookie on a different path.
			assert.Loosely(t, deleted, should.HaveLength(1))
			assert.Loosely(t, deleted[0].Path, should.Equal(internal.LimitedCookiePath))

			// Handed out 1 access token thus far.
			assert.Loosely(t, provider.AccessTokensMinted(), should.Equal(1))

			t.Run("Code reuse is forbidden", func(t *ftt.Test) {
				// Trying to use the authorization code again fails.
				resp := call(methodV1.callbackHandler, "dest.example.com", &url.URL{
					RawQuery: callbackRawQueryV1,
				}, nil)
				assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusBadRequest))
			})

			t.Run("No cookies => method is skipped", func(t *ftt.Test) {
				user, session, err := methodV1.Authenticate(ctx, phonyRequest(nil))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.BeNil)
				assert.Loosely(t, session, should.BeNil)
			})

			t.Run("Good cookie works", func(t *ftt.Test) {
				user, session, err := methodV1.Authenticate(ctx, phonyRequest(cookieV1))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.Match(&auth.User{
					Identity: identity.Identity("user:" + provider.UserEmail),
					Email:    provider.UserEmail,
					Name:     provider.UserName,
					Picture:  provider.UserPicture,
				}))
				assert.Loosely(t, session, should.NotBeNil)

				// Can grab the stored access token.
				tok, err := session.AccessToken(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tok.AccessToken, should.Equal("access_token_1"))
				assert.Loosely(t, tok.Expiry.Sub(testclock.TestRecentTimeUTC), should.Equal(time.Hour))

				// Can grab the stored ID token.
				tok, err = session.IDToken(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tok.AccessToken, should.HavePrefix("eyJhbG")) // JWT header
				assert.Loosely(t, tok.Expiry.Sub(testclock.TestRecentTimeUTC), should.Equal(time.Hour))
			})

			t.Run("Malformed cookie is ignored", func(t *ftt.Test) {
				user, session, err := methodV1.Authenticate(ctx, phonyRequest(&http.Cookie{
					Name:  cookieV1.Name,
					Value: cookieV1.Value[:20],
				}))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.BeNil)
				assert.Loosely(t, session, should.BeNil)
			})

			t.Run("Missing datastore session", func(t *ftt.Test) {
				methodV1.Sessions.(*datastore.Store).Namespace = "another"
				user, session, err := methodV1.Authenticate(ctx, phonyRequest(cookieV1))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.BeNil)
				assert.Loosely(t, session, should.BeNil)
			})

			t.Run("After short-lived tokens expire", func(t *ftt.Test) {
				tc.Add(2 * time.Hour)

				t.Run("Session refresh OK", func(t *ftt.Test) {
					// The session is still valid.
					user, session, err := methodV1.Authenticate(ctx, phonyRequest(cookieV1))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, user, should.NotBeNil)
					assert.Loosely(t, session, should.NotBeNil)

					// Tokens have been refreshed.
					assert.Loosely(t, provider.AccessTokensMinted(), should.Equal(2))

					// auth.Session returns the refreshed token.
					tok, err := session.AccessToken(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, tok.AccessToken, should.Equal("access_token_2"))
					assert.Loosely(t, tok.Expiry.Sub(testclock.TestRecentTimeUTC), should.Equal(3*time.Hour))

					// No need to refresh anymore.
					user, session, err = methodV1.Authenticate(ctx, phonyRequest(cookieV1))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, user, should.NotBeNil)
					assert.Loosely(t, session, should.NotBeNil)
					assert.Loosely(t, provider.AccessTokensMinted(), should.Equal(2))
				})

				t.Run("Session refresh transient fail", func(t *ftt.Test) {
					provider.TransientErr = errors.New("boom")

					_, _, err := methodV1.Authenticate(ctx, phonyRequest(cookieV1))
					assert.Loosely(t, err, should.NotBeNil)
					assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
				})

				t.Run("Session refresh fatal fail", func(t *ftt.Test) {
					provider.RefreshToken = "another-token"

					// Refresh fails and closes the session.
					user, session, err := methodV1.Authenticate(ctx, phonyRequest(cookieV1))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, user, should.BeNil)
					assert.Loosely(t, session, should.BeNil)

					// Using the closed session is unsuccessful.
					user, session, err = methodV1.Authenticate(ctx, phonyRequest(cookieV1))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, user, should.BeNil)
					assert.Loosely(t, session, should.BeNil)
				})
			})

			t.Run("Logout works", func(t *ftt.Test) {
				logoutURL, err := methodV1.LogoutURL(ctx, "/some/dest")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, logoutURL, should.Equal("/auth/openid/logout?r=%2Fsome%2Fdest"))
				parsed, _ := url.Parse(logoutURL)

				resp := call(methodV1.logoutHandler, "primary.example.com", parsed, http.Header{
					"Cookie": {fmt.Sprintf("%s=%s", cookieV1.Name, cookieV1.Value)},
				})

				// Got a redirect to the final destination URL.
				assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusFound))
				assert.Loosely(t, resp.Header.Get("Location"), should.Equal("/some/dest"))

				// Cookies are removed.
				cookies := resp.Cookies()
				paths := []string{}
				for _, c := range cookies {
					assert.Loosely(t, c.Name, should.Equal(internal.SessionCookieName))
					assert.Loosely(t, c.Value, should.Equal("deleted"))
					assert.Loosely(t, c.MaxAge, should.Equal(-1))
					paths = append(paths, c.Path)
				}
				assert.Loosely(t, paths, should.Match([]string{internal.UnlimitedCookiePath, internal.LimitedCookiePath}))

				// It also no longer works.
				user, session, err := methodV1.Authenticate(ctx, phonyRequest(cookieV1))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.BeNil)
				assert.Loosely(t, session, should.BeNil)

				// The refresh token was not revoked.
				assert.Loosely(t, provider.Revoked, should.BeNil)

				// Hitting logout again (resending the cookie) succeeds.
				resp = call(methodV1.logoutHandler, "primary.example.com", parsed, http.Header{
					"Cookie": {fmt.Sprintf("%s=%s", cookieV1.Name, cookieV1.Value)},
				})
				assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusFound))
				assert.Loosely(t, resp.Header.Get("Location"), should.Equal("/some/dest"))
				assert.Loosely(t, provider.Revoked, should.BeNil)
			})

			t.Run("Add additional optional scope works", func(t *ftt.Test) {
				methodV2 := makeMethod([]string{"scope1"}, []string{"scope2", "scope3"})

				user, session, err := methodV2.Authenticate(ctx, phonyRequest(cookieV1))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.Match(&auth.User{
					Identity: identity.Identity("user:" + provider.UserEmail),
					Email:    provider.UserEmail,
					Name:     provider.UserName,
					Picture:  provider.UserPicture,
				}))
				assert.Loosely(t, session, should.NotBeNil)
			})

			t.Run("Promote optional scope works", func(t *ftt.Test) {
				methodV2 := makeMethod([]string{"scope1", "scope2"}, nil)

				user, session, err := methodV2.Authenticate(ctx, phonyRequest(cookieV1))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.Match(&auth.User{
					Identity: identity.Identity("user:" + provider.UserEmail),
					Email:    provider.UserEmail,
					Name:     provider.UserName,
					Picture:  provider.UserPicture,
				}))
				assert.Loosely(t, session, should.NotBeNil)
			})

			t.Run("Add additional required scope invalidates old sessions", func(t *ftt.Test) {
				methodV2 := makeMethod([]string{"scope1", "scope3"}, []string{"scope2"})

				user, session, err := methodV2.Authenticate(ctx, phonyRequest(cookieV1))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.BeNil)
				assert.Loosely(t, session, should.BeNil)

				// The cookie no longer works with the old method.
				user, session, err = methodV1.Authenticate(ctx, phonyRequest(cookieV1))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.BeNil)
				assert.Loosely(t, session, should.BeNil)
			})

			t.Run("Additional scopes are decided during login not callback", func(t *ftt.Test) {
				methodV2 := makeMethod([]string{"scope1"}, []string{"scope2", "scope3"})
				methodV3 := makeMethod([]string{"scope1", "scope3"}, []string{"scope2"})

				// User hit the login handle in the v1 but the callback is handled by
				// v2.
				callbackRawQueryV1 := performLogin(methodV1)
				cookieV1, _ := performCallback(methodV2, callbackRawQueryV1)

				// Cookies produced by login requests in methodV1 does not have the
				// added scope.
				user, session, err := methodV3.Authenticate(ctx, phonyRequest(cookieV1))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.BeNil)
				assert.Loosely(t, session, should.BeNil)

				// User hit the login handle in the v2 but the callback is handled by
				// v1.
				callbackRawQueryV2 := performLogin(methodV2)
				cookieV2, _ := performCallback(methodV1, callbackRawQueryV2)

				// Cookies produced by login requests in methodV2 does have the added
				// scope.
				user, session, err = methodV3.Authenticate(ctx, phonyRequest(cookieV2))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, user, should.Match(&auth.User{
					Identity: identity.Identity("user:" + provider.UserEmail),
					Email:    provider.UserEmail,
					Name:     provider.UserName,
					Picture:  provider.UserPicture,
				}))
				assert.Loosely(t, session, should.NotBeNil)
			})
		})

		t.Run("LimitCookieExposure cookie works", func(t *ftt.Test) {
			method := makeMethod([]string{"scope1"}, []string{"scope2"})
			method.LimitCookieExposure = true

			cookie, deleted := performCallback(method, performLogin(method))

			// Set the cookie on the correct path.
			assert.Loosely(t, cookie.Path, should.Equal(internal.LimitedCookiePath))
			assert.Loosely(t, cookie.SameSite, should.Equal(http.SameSiteStrictMode))

			// Removed a potentially stale cookie on a different path.
			assert.Loosely(t, deleted, should.HaveLength(1))
			assert.Loosely(t, deleted[0].Path, should.Equal(internal.UnlimitedCookiePath))

			user, _, _ := method.Authenticate(ctx, phonyRequest(cookie))
			assert.Loosely(t, user, should.Match(&auth.User{
				Identity: identity.Identity("user:" + provider.UserEmail),
				Email:    provider.UserEmail,
				Name:     provider.UserName,
				Picture:  provider.UserPicture,
			}))
		})

		t.Run("State endpoint works", func(t *ftt.Test) {
			r := router.New()
			r.Use(router.MiddlewareChain{
				func(rc *router.Context, next router.Handler) {
					rc.Request = rc.Request.WithContext(ctx)
					next(rc)
				},
			})
			methodV1.InstallHandlers(r, nil)

			callState := func(cookie *http.Cookie) (code int, state *auth.StateEndpointResponse) {
				rw := httptest.NewRecorder()
				req := httptest.NewRequest("GET", stateURL, nil)
				if cookie != nil {
					req.Header.Add("Cookie", fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
				}
				req.Header.Add("Sec-Fetch-Site", "same-origin")
				r.ServeHTTP(rw, req)
				res := rw.Result()
				code = res.StatusCode
				if code == 200 {
					state = &auth.StateEndpointResponse{}
					assert.Loosely(t, json.NewDecoder(res.Body).Decode(&state), should.BeNil)
				}
				return
			}

			goodCookie, _ := performCallback(methodV1, performLogin(methodV1))
			assert.Loosely(t, provider.AccessTokensMinted(), should.Equal(1))
			tc.Add(30 * time.Minute)

			t.Run("No cookie", func(t *ftt.Test) {
				code, state := callState(nil)
				assert.Loosely(t, code, should.Equal(200))
				assert.Loosely(t, state, should.Match(&auth.StateEndpointResponse{Identity: "anonymous:anonymous"}))
			})

			t.Run("Valid cookie", func(t *ftt.Test) {
				code, state := callState(goodCookie)
				assert.Loosely(t, code, should.Equal(200))
				assert.Loosely(t, state, should.Match(&auth.StateEndpointResponse{
					Identity:             "user:someone@example.com",
					Email:                "someone@example.com",
					Picture:              "https://example.com/picture",
					AccessToken:          "access_token_1",
					AccessTokenExpiry:    testclock.TestRecentTimeUTC.Add(time.Hour).Unix(),
					AccessTokenExpiresIn: 1800,
					IDToken:              state.IDToken, // checked separately
					IDTokenExpiry:        testclock.TestRecentTimeUTC.Add(time.Hour).Unix(),
					IDTokenExpiresIn:     1800,
				}))
				assert.Loosely(t, state.IDToken, should.HavePrefix("eyJhbG")) // JWT header

				// Still only 1 token minted overall.
				assert.Loosely(t, provider.AccessTokensMinted(), should.Equal(1))
			})

			t.Run("Refreshes tokens", func(t *ftt.Test) {
				tc.Add(time.Hour) // make sure existing tokens expire

				code, state := callState(goodCookie)
				assert.Loosely(t, code, should.Equal(200))
				assert.Loosely(t, state, should.Match(&auth.StateEndpointResponse{
					Identity:             "user:someone@example.com",
					Email:                "someone@example.com",
					Picture:              "https://example.com/picture",
					AccessToken:          "access_token_2",
					AccessTokenExpiry:    testclock.TestRecentTimeUTC.Add(2*time.Hour + 30*time.Minute).Unix(),
					AccessTokenExpiresIn: 3600,
					IDToken:              state.IDToken, // checked separately
					IDTokenExpiry:        testclock.TestRecentTimeUTC.Add(2*time.Hour + 30*time.Minute).Unix(),
					IDTokenExpiresIn:     3600,
				}))
				assert.Loosely(t, state.IDToken, should.HavePrefix("eyJhbG")) // JWT header

				// Minted a new token.
				assert.Loosely(t, provider.AccessTokensMinted(), should.Equal(2))
			})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

const (
	fakeSigningKeyID = "signing-key"
	fakeIssuer       = "https://issuer.example.com"
)

type openIDProviderFake struct {
	ExpectedClientID     string
	ExpectedClientSecret string
	ExpectedRedirectURI  string

	UserSub     string
	UserEmail   string
	UserName    string
	UserPicture string

	RefreshToken string
	Revoked      []string

	TransientErr error

	t             testing.TB
	srv           *httptest.Server
	signer        *signingtest.Signer
	nextAccessTok int64
}

func (f *openIDProviderFake) Init(ctx context.Context, t testing.TB) {
	r := router.New()
	r.GET("/discovery", nil, f.discoveryHandler)
	r.GET("/jwks", nil, f.jwksHandler)
	r.POST("/token", nil, f.tokenHandler)
	r.POST("/revocation", nil, f.revocationHandler)
	f.t = t
	f.srv = httptest.NewServer(r)
	f.signer = signingtest.NewSigner(nil)
}

func (f *openIDProviderFake) Close() {
	f.srv.Close()
}

func (f *openIDProviderFake) Host() string {
	return strings.TrimPrefix(f.srv.URL, "http://")
}

func (f *openIDProviderFake) DiscoveryURL() string {
	return f.srv.URL + "/discovery"
}

func (f *openIDProviderFake) CallbackRawQuery(q url.Values) string {
	assert.That(f.t, q.Get("client_id"), should.Equal(f.ExpectedClientID))
	assert.That(f.t, q.Get("redirect_uri"), should.Equal(f.ExpectedRedirectURI))
	assert.That(f.t, q.Get("nonce"), should.NotEqual(""))
	assert.That(f.t, q.Get("code_challenge_method"), should.Equal("S256"))
	assert.That(f.t, q.Get("code_challenge"), should.NotEqual(""))

	// Remember the nonce and the code verifier challenge just by encoding them
	// in the resulting authorization code. This code is opaque to the ID
	// provider clients. It will be passed back to us in /token handler.
	blob, _ := json.Marshal(map[string]string{
		"nonce":          q.Get("nonce"),
		"code_challenge": q.Get("code_challenge"),
	})
	authCode := base64.RawStdEncoding.EncodeToString(blob)

	return fmt.Sprintf("code=%s&state=%s", authCode, q.Get("state"))
}

func (f *openIDProviderFake) AccessTokensMinted() int64 {
	return atomic.LoadInt64(&f.nextAccessTok)
}

func (f *openIDProviderFake) discoveryHandler(ctx *router.Context) {
	json.NewEncoder(ctx.Writer).Encode(map[string]string{
		"issuer":                 fakeIssuer,
		"authorization_endpoint": f.srv.URL + "/authorization", // not actually called in test
		"token_endpoint":         f.srv.URL + "/token",
		"revocation_endpoint":    f.srv.URL + "/revocation",
		"jwks_uri":               f.srv.URL + "/jwks",
	})
}

func (f *openIDProviderFake) jwksHandler(ctx *router.Context) {
	keys := jwksForTest(fakeSigningKeyID, &f.signer.KeyForTest().PublicKey)
	json.NewEncoder(ctx.Writer).Encode(keys)
}

func (f *openIDProviderFake) tokenHandler(ctx *router.Context) {
	if f.TransientErr != nil {
		http.Error(ctx.Writer, f.TransientErr.Error(), 500)
		return
	}
	if !f.checkClient(ctx) {
		return
	}
	if ctx.Request.FormValue("redirect_uri") != f.ExpectedRedirectURI {
		http.Error(ctx.Writer, "bad redirect URI", 400)
		return
	}

	switch ctx.Request.FormValue("grant_type") {
	case "authorization_code":
		code := ctx.Request.FormValue("code")
		codeVerifier := ctx.Request.FormValue("code_verifier")

		// Parse the code generated in CallbackRawQuery.
		blob, err := base64.RawStdEncoding.DecodeString(code)
		if err != nil {
			http.Error(ctx.Writer, "bad code base64", 400)
			return
		}
		var encodedParams struct {
			Nonce         string `json:"nonce"`
			CodeChallenge string `json:"code_challenge"`
		}
		if err := json.Unmarshal(blob, &encodedParams); err != nil {
			http.Error(ctx.Writer, "bad code JSON", 400)
			return
		}

		// Verify the given `code_verifier` matches `code_challenge`.
		if internal.DeriveCodeChallenge(codeVerifier) != encodedParams.CodeChallenge {
			http.Error(ctx.Writer, "bad code_challenge", 400)
			return
		}

		// All is good! Generate tokens.
		json.NewEncoder(ctx.Writer).Encode(map[string]any{
			"expires_in":    3600,
			"refresh_token": f.RefreshToken,
			"access_token":  f.genAccessToken(),
			"id_token":      f.genIDToken(ctx.Request.Context(), encodedParams.Nonce),
		})

	case "refresh_token":
		refreshToken := ctx.Request.FormValue("refresh_token")
		if refreshToken != f.RefreshToken {
			http.Error(ctx.Writer, "bad refresh token", 400)
			return
		}
		json.NewEncoder(ctx.Writer).Encode(map[string]any{
			"expires_in":   3600,
			"access_token": f.genAccessToken(),
			"id_token":     f.genIDToken(ctx.Request.Context(), ""),
		})

	default:
		http.Error(ctx.Writer, "unknown grant_type", 400)
	}
}

func (f *openIDProviderFake) revocationHandler(ctx *router.Context) {
	if !f.checkClient(ctx) {
		return
	}
	f.Revoked = append(f.Revoked, ctx.Request.FormValue("token"))
}

func (f *openIDProviderFake) checkClient(ctx *router.Context) bool {
	if ctx.Request.FormValue("client_id") != f.ExpectedClientID {
		http.Error(ctx.Writer, "bad client ID", 400)
		return false
	}
	if ctx.Request.FormValue("client_secret") != f.ExpectedClientSecret {
		http.Error(ctx.Writer, "bad client secret", 400)
		return false
	}
	return true
}

func (f *openIDProviderFake) genAccessToken() string {
	count := atomic.AddInt64(&f.nextAccessTok, 1)
	return fmt.Sprintf("access_token_%d", count)
}

func (f *openIDProviderFake) genIDToken(ctx context.Context, nonce string) string {
	body, err := json.Marshal(openid.IDToken{
		Iss:           fakeIssuer,
		EmailVerified: true,
		Sub:           f.UserSub,
		Email:         f.UserEmail,
		Name:          f.UserName,
		Picture:       f.UserPicture,
		Aud:           f.ExpectedClientID,
		Iat:           clock.Now(ctx).Unix(),
		Exp:           clock.Now(ctx).Add(time.Hour).Unix(),
		Nonce:         nonce,
	})
	if err != nil {
		panic(err)
	}
	return jwtForTest(ctx, body, fakeSigningKeyID, f.signer)
}

////////////////////////////////////////////////////////////////////////////////

func jwksForTest(keyID string, pubKey *rsa.PublicKey) *openid.JSONWebKeySetStruct {
	modulus := pubKey.N.Bytes()
	exp := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(exp, uint32(pubKey.E))
	return &openid.JSONWebKeySetStruct{
		Keys: []openid.JSONWebKeyStruct{
			{
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
				Kid: keyID,
				N:   base64.RawURLEncoding.EncodeToString(modulus),
				E:   base64.RawURLEncoding.EncodeToString(exp),
			},
		},
	}
}

func jwtForTest(ctx context.Context, body []byte, keyID string, signer *signingtest.Signer) string {
	b64hdr := base64.RawURLEncoding.EncodeToString([]byte(
		fmt.Sprintf(`{"alg": "RS256","kid": "%s"}`, keyID)))
	b64bdy := base64.RawURLEncoding.EncodeToString(body)
	_, sig, err := signer.SignBytes(ctx, []byte(b64hdr+"."+b64bdy))
	if err != nil {
		panic(err)
	}
	return b64hdr + "." + b64bdy + "." + base64.RawURLEncoding.EncodeToString(sig)
}
