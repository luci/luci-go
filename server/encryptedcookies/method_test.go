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
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = authtest.MockAuthConfig(ctx)
		ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
			cfg.AEADProvider = func(context.Context) tink.AEAD {
				return ae
			}
			return cfg
		})

		method := &AuthMethod{
			OpenIDConfig: func(context.Context) (*OpenIDConfig, error) {
				return &OpenIDConfig{
					DiscoveryURL: provider.DiscoveryURL(),
					ClientID:     provider.ExpectedClientID,
					ClientSecret: provider.ExpectedClientSecret,
					RedirectURI:  provider.ExpectedRedirectURI,
				}, nil
			},
			Sessions: &datastore.Store{},
		}

		call := func(h router.Handler, host string, url *url.URL) *http.Response {
			rw := httptest.NewRecorder()
			h(&router.Context{
				Context: ctx,
				Request: &http.Request{
					URL:  url,
					Host: host,
				},
				Writer: rw,
			})
			return rw.Result()
		}

		Convey("Full flow", func() {
			loginURL, err := method.LoginURL(ctx, "/some/dest")
			So(err, ShouldBeNil)
			So(loginURL, ShouldEqual, "/auth/openid/login?r=%2Fsome%2Fdest")
			parsed, _ := url.Parse(loginURL)

			// Hitting the generated login URL generates a redirect to the provider.
			resp := call(method.loginHandler, "dest.example.com", parsed)
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
			})

			// We got a redirect to "dest.example.com".
			So(resp.StatusCode, ShouldEqual, http.StatusFound)
			So(resp.Header.Get("Location"), ShouldEqual, "https://dest.example.com?"+callbackRawQuery)

			// Now hitting the same callback on "dest.example.com".
			resp = call(method.callbackHandler, "dest.example.com", &url.URL{
				RawQuery: callbackRawQuery,
			})

			// Got a redirect to the final destination URL.
			So(resp.StatusCode, ShouldEqual, http.StatusFound)
			So(resp.Header.Get("Location"), ShouldEqual, "/some/dest")

			// And we've got some session cookie!
			setCookie := resp.Header.Get("Set-Cookie")
			So(setCookie, ShouldNotEqual, "")
			cookie := strings.Split(setCookie, ";")[0]

			Convey("Code reuse is forbidden", func() {
				// Trying to use the authorization code again fails.
				resp := call(method.callbackHandler, "dest.example.com", &url.URL{
					RawQuery: callbackRawQuery,
				})
				So(resp.StatusCode, ShouldEqual, http.StatusBadRequest)
			})

			Convey("Session cookies", func() {
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
					So(session, ShouldBeNil)
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
			})

			Convey("After short-lived tokens expire", func() {
				tc.Add(2 * time.Hour)

				Convey("Session refresh OK", func() {
					// The session is still valid.
					req, _ := http.NewRequest("GET", "https://dest.example.com/phony", nil)
					req.Header.Add("Cookie", cookie)
					_, _, err := method.Authenticate(ctx, req)
					So(err, ShouldBeNil)

					// Tokens have been refreshed.
					// TODO: check
				})

				Convey("Session refresh transient fail", func() {
					// TODO
				})

				Convey("Session refresh fatal fail", func() {
					// TODO
				})
			})

			Convey("Logout works", func() {
				// Logout clears the cookie and invalidates the session.
				// TODO: check once implemented.
			})
		})
	})
}

func TestNormalizeURL(t *testing.T) {
	t.Parallel()

	Convey("Normalizes good URLs", t, func(ctx C) {
		cases := []struct {
			in  string
			out string
		}{
			{"", "/"},
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
				ctx.Printf("Failed while checking %q\n", c.in)
				So(err, ShouldBeNil)
			}
			So(out, ShouldEqual, c.out)
		}
	})

	Convey("Rejects bad URLs", t, func(ctx C) {
		cases := []string{
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
				ctx.Printf("Didn't fail while testing %q\n", c)
			}
			So(err, ShouldNotBeNil)
		}
	})
}

////////////////////////////////////////////////////////////////////////////////

const (
	fakeSigningKeyID = "signing-key"
	fakeIssuer       = "https://issuer.example.com"
	fakeRefreshToken = "secret-refresh-token"
)

type openIDProviderFake struct {
	ExpectedClientID     string
	ExpectedClientSecret string
	ExpectedRedirectURI  string

	UserSub     string
	UserEmail   string
	UserName    string
	UserPicture string

	c             C
	srv           *httptest.Server
	signer        *signingtest.Signer
	nextAccessTok int64
}

func (f *openIDProviderFake) Init(ctx context.Context, c C) {
	r := router.NewWithRootContext(ctx)
	r.GET("/discovery", router.MiddlewareChain{}, f.discoveryHandler)
	r.GET("/jwks", router.MiddlewareChain{}, f.jwksHandler)
	r.POST("/token", router.MiddlewareChain{}, f.tokenHandler)
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

func (f *openIDProviderFake) discoveryHandler(ctx *router.Context) {
	json.NewEncoder(ctx.Writer).Encode(map[string]string{
		"issuer":                 fakeIssuer,
		"authorization_endpoint": f.srv.URL + "/authorization", // not actually called in test
		"token_endpoint":         f.srv.URL + "/token",
		"jwks_uri":               f.srv.URL + "/jwks",
	})
}

func (f *openIDProviderFake) jwksHandler(ctx *router.Context) {
	keys := jwksForTest(fakeSigningKeyID, &f.signer.KeyForTest().PublicKey)
	json.NewEncoder(ctx.Writer).Encode(keys)
}

func (f *openIDProviderFake) tokenHandler(ctx *router.Context) {
	clientID := ctx.Request.FormValue("client_id")
	if clientID != f.ExpectedClientID {
		http.Error(ctx.Writer, "bad client ID", 400)
		return
	}
	clientSecret := ctx.Request.FormValue("client_secret")
	if clientSecret != f.ExpectedClientSecret {
		http.Error(ctx.Writer, "bad client secret", 400)
		return
	}
	redirectURI := ctx.Request.FormValue("redirect_uri")
	if redirectURI != f.ExpectedRedirectURI {
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

		now := clock.Now(ctx.Context)

		// All is good! Generate tokens.
		json.NewEncoder(ctx.Writer).Encode(map[string]interface{}{
			"expires_in":    3600,
			"refresh_token": fakeRefreshToken,
			"access_token":  f.genAccessToken(),
			"id_token": f.signedToken(ctx.Context, openid.IDToken{
				Iss:           fakeIssuer,
				EmailVerified: true,
				Sub:           f.UserSub,
				Email:         f.UserEmail,
				Name:          f.UserName,
				Picture:       f.UserPicture,
				Aud:           clientID,
				Iat:           now.Unix(),
				Exp:           now.Add(time.Hour).Unix(),
				Nonce:         encodedParams.Nonce,
			}),
		})

	case "refresh_token":
		// TODO: implement to test the token refresh flow once it exists.

	default:
		http.Error(ctx.Writer, "unknown grant_type", 400)
	}
}

func (f *openIDProviderFake) genAccessToken() string {
	count := atomic.AddInt64(&f.nextAccessTok, 1)
	return fmt.Sprintf("access_token_%d", count)
}

func (f *openIDProviderFake) signedToken(ctx context.Context, tok openid.IDToken) string {
	body, err := json.Marshal(tok)
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
