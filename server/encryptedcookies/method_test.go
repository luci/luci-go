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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/encryptedcookies/internal"
	"go.chromium.org/luci/server/encryptedcookies/session/datastore"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMethod(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		// Instantiate ID provider with mocked time only. It doesn't need other
		// context features.
		ctx := context.Background()
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)
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
		provider.Init(ctx, c)
		defer provider.Close()

		// Primary encryption keys used to encrypt cookies.
		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		So(err, ShouldBeNil)
		ae, err := aead.New(kh)
		So(err, ShouldBeNil)

		// Install the rest of the context stuff used by AuthMethod.
		ctx = memory.Use(ctx)
		ctx = gologger.StdConfig.Use(ctx)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = authtest.MockAuthConfig(ctx)

		method := &AuthMethod{
			OpenIDConfig: func(context.Context) (*OpenIDConfig, error) {
				return &OpenIDConfig{
					DiscoveryURL: provider.DiscoveryURL(),
					ClientID:     provider.ExpectedClientID,
					ClientSecret: provider.ExpectedClientSecret,
					RedirectURI:  provider.ExpectedRedirectURI,
				}, nil
			},
			AEADProvider: func(context.Context) tink.AEAD { return ae },
			Sessions:     &datastore.Store{},
		}

		call := func(h router.Handler, host string, url *url.URL, header http.Header) *http.Response {
			rw := httptest.NewRecorder()
			h(&router.Context{
				Context: ctx,
				Request: &http.Request{
					URL:    url,
					Host:   host,
					Header: header,
				},
				Writer: rw,
			})
			return rw.Result()
		}

		Convey("Full flow", func() {
			So(method.Warmup(ctx), ShouldBeNil)

			loginURL, err := method.LoginURL(ctx, "/some/dest")
			So(err, ShouldBeNil)
			So(loginURL, ShouldEqual, "/auth/openid/login?r=%2Fsome%2Fdest")
			parsed, _ := url.Parse(loginURL)

			// Hitting the generated login URL generates a redirect to the provider.
			resp := call(method.loginHandler, "dest.example.com", parsed, nil)
			So(resp.StatusCode, ShouldEqual, http.StatusFound)
			authURL, _ := url.Parse(resp.Header.Get("Location"))
			So(authURL.Host, ShouldEqual, provider.Host())
			So(authURL.Path, ShouldEqual, "/authorization")

			// After the user logs in, the provider generates a redirect to the
			// callback URI with some query parameters.
			callbackRawQuery := provider.CallbackRawQuery(authURL.Query())

			// Provider calls us back on our primary host (not "dest.example.com").
			resp = call(method.callbackHandler, "primary.example.com", &url.URL{
				RawQuery: callbackRawQuery,
			}, nil)

			// We got a redirect to "dest.example.com".
			So(resp.StatusCode, ShouldEqual, http.StatusFound)
			So(resp.Header.Get("Location"), ShouldEqual, "https://dest.example.com?"+callbackRawQuery)

			// Now hitting the same callback on "dest.example.com".
			resp = call(method.callbackHandler, "dest.example.com", &url.URL{
				RawQuery: callbackRawQuery,
			}, nil)

			// Got a redirect to the final destination URL.
			So(resp.StatusCode, ShouldEqual, http.StatusFound)
			So(resp.Header.Get("Location"), ShouldEqual, "/some/dest")

			// And we've got some session cookie!
			setCookie := resp.Header.Get("Set-Cookie")
			So(setCookie, ShouldNotEqual, "")
			cookie := strings.Split(setCookie, ";")[0]

			// Handed out 1 access token thus far.
			So(provider.AccessTokensMinted(), ShouldEqual, 1)

			Convey("Code reuse is forbidden", func() {
				// Trying to use the authorization code again fails.
				resp := call(method.callbackHandler, "dest.example.com", &url.URL{
					RawQuery: callbackRawQuery,
				}, nil)
				So(resp.StatusCode, ShouldEqual, http.StatusBadRequest)
			})

			phonyRequest := func(cookie string) *http.Request {
				req, _ := http.NewRequest("GET", "https://dest.example.com/phony", nil)
				if cookie != "" {
					req.Header.Add("Cookie", cookie)
				}
				return req
			}

			Convey("No cookies => method is skipped", func() {
				user, session, err := method.Authenticate(ctx, phonyRequest(""))
				So(err, ShouldBeNil)
				So(user, ShouldBeNil)
				So(session, ShouldBeNil)
			})

			Convey("Good cookie works", func() {
				user, session, err := method.Authenticate(ctx, phonyRequest(cookie))
				So(err, ShouldBeNil)
				So(user, ShouldResemble, &auth.User{
					Identity: identity.Identity("user:" + provider.UserEmail),
					Email:    provider.UserEmail,
					Name:     provider.UserName,
					Picture:  provider.UserPicture,
				})
				So(session, ShouldNotBeNil)

				// Can grab the stored access token.
				tok, err := session.AccessToken(ctx)
				So(err, ShouldBeNil)
				So(tok.AccessToken, ShouldEqual, "access_token_1")
				So(tok.Expiry.Sub(testclock.TestTimeUTC), ShouldEqual, time.Hour)

				// Can grab the stored ID token.
				tok, err = session.IDToken(ctx)
				So(err, ShouldBeNil)
				So(tok.AccessToken, ShouldStartWith, "eyJhbG") // JWT header
				So(tok.Expiry.Sub(testclock.TestTimeUTC), ShouldEqual, time.Hour)
			})

			Convey("Malformed cookie is ignored", func() {
				user, session, err := method.Authenticate(ctx, phonyRequest(cookie[:20]+"aaaaaa"+cookie[26:]))
				So(err, ShouldBeNil)
				So(user, ShouldBeNil)
				So(session, ShouldBeNil)
			})

			Convey("Missing datastore session", func() {
				method.Sessions.(*datastore.Store).Namespace = "another"
				user, session, err := method.Authenticate(ctx, phonyRequest(cookie))
				So(err, ShouldBeNil)
				So(user, ShouldBeNil)
				So(session, ShouldBeNil)
			})

			Convey("After short-lived tokens expire", func() {
				tc.Add(2 * time.Hour)

				Convey("Session refresh OK", func() {
					// The session is still valid.
					user, session, err := method.Authenticate(ctx, phonyRequest(cookie))
					So(err, ShouldBeNil)
					So(user, ShouldNotBeNil)
					So(session, ShouldNotBeNil)

					// Tokens have been refreshed.
					So(provider.AccessTokensMinted(), ShouldEqual, 2)

					// auth.Session returns the refreshed token.
					tok, err := session.AccessToken(ctx)
					So(err, ShouldBeNil)
					So(tok.AccessToken, ShouldEqual, "access_token_2")
					So(tok.Expiry.Sub(testclock.TestTimeUTC), ShouldEqual, 3*time.Hour)

					// No need to refresh anymore.
					user, session, err = method.Authenticate(ctx, phonyRequest(cookie))
					So(err, ShouldBeNil)
					So(user, ShouldNotBeNil)
					So(session, ShouldNotBeNil)
					So(provider.AccessTokensMinted(), ShouldEqual, 2)
				})

				Convey("Session refresh transient fail", func() {
					provider.TransientErr = errors.New("boom")

					_, _, err := method.Authenticate(ctx, phonyRequest(cookie))
					So(err, ShouldNotBeNil)
					So(transient.Tag.In(err), ShouldBeTrue)
				})

				Convey("Session refresh fatal fail", func() {
					provider.RefreshToken = "another-token"

					// Refresh fails and closes the session.
					user, session, err := method.Authenticate(ctx, phonyRequest(cookie))
					So(err, ShouldBeNil)
					So(user, ShouldBeNil)
					So(session, ShouldBeNil)

					// Using the closed session is unsuccessful.
					user, session, err = method.Authenticate(ctx, phonyRequest(cookie))
					So(err, ShouldBeNil)
					So(user, ShouldBeNil)
					So(session, ShouldBeNil)
				})
			})

			Convey("Logout works", func() {
				logoutURL, err := method.LogoutURL(ctx, "/some/dest")
				So(err, ShouldBeNil)
				So(logoutURL, ShouldEqual, "/auth/openid/logout?r=%2Fsome%2Fdest")
				parsed, _ := url.Parse(logoutURL)

				resp := call(method.logoutHandler, "primary.example.com", parsed, http.Header{
					"Cookie": {cookie},
				})

				// Got a redirect to the final destination URL.
				So(resp.StatusCode, ShouldEqual, http.StatusFound)
				So(resp.Header.Get("Location"), ShouldEqual, "/some/dest")

				// The cookie is removed.
				So(resp.Header.Get("Set-Cookie"), ShouldStartWith, "LUCISID=deleted")

				// It also no longer works.
				user, session, err := method.Authenticate(ctx, phonyRequest(cookie))
				So(err, ShouldBeNil)
				So(user, ShouldBeNil)
				So(session, ShouldBeNil)

				// The refresh token was not revoked.
				So(provider.Revoked, ShouldBeNil)

				// Hitting logout again (resending the cookie) succeeds.
				resp = call(method.logoutHandler, "primary.example.com", parsed, http.Header{
					"Cookie": {cookie},
				})
				So(resp.StatusCode, ShouldEqual, http.StatusFound)
				So(resp.Header.Get("Location"), ShouldEqual, "/some/dest")
				So(provider.Revoked, ShouldBeNil)
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

	c             C
	srv           *httptest.Server
	signer        *signingtest.Signer
	nextAccessTok int64
}

func (f *openIDProviderFake) Init(ctx context.Context, c C) {
	r := router.NewWithRootContext(ctx)
	r.GET("/discovery", nil, f.discoveryHandler)
	r.GET("/jwks", nil, f.jwksHandler)
	r.POST("/token", nil, f.tokenHandler)
	r.POST("/revocation", nil, f.revocationHandler)
	f.c = c
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
	f.c.So(q.Get("client_id"), ShouldEqual, f.ExpectedClientID)
	f.c.So(q.Get("redirect_uri"), ShouldEqual, f.ExpectedRedirectURI)
	f.c.So(q.Get("nonce"), ShouldNotEqual, "")
	f.c.So(q.Get("code_challenge_method"), ShouldEqual, "S256")
	f.c.So(q.Get("code_challenge"), ShouldNotEqual, "")

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
		json.NewEncoder(ctx.Writer).Encode(map[string]interface{}{
			"expires_in":    3600,
			"refresh_token": f.RefreshToken,
			"access_token":  f.genAccessToken(),
			"id_token":      f.genIDToken(ctx.Context, encodedParams.Nonce),
		})

	case "refresh_token":
		refreshToken := ctx.Request.FormValue("refresh_token")
		if refreshToken != f.RefreshToken {
			http.Error(ctx.Writer, "bad refresh token", 400)
			return
		}
		json.NewEncoder(ctx.Writer).Encode(map[string]interface{}{
			"expires_in":   3600,
			"access_token": f.genAccessToken(),
			"id_token":     f.genIDToken(ctx.Context, ""),
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
