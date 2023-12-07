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

package openid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGoogleIDTokenAuthMethod(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = authtest.MockAuthConfig(ctx)
	ctx, _ = testclock.UseTime(ctx, time.Unix(1442540000, 0))

	provider := &fakeIdentityProvider{
		Signer:       signingtest.NewSigner(nil),
		SigningKeyID: "signing-key",
		Issuer:       "https://issuer.example.com",
	}
	provider.start()
	defer provider.stop()

	const fakeHost = "fake-host.example.com"

	method := GoogleIDTokenAuthMethod{
		Audience:      []string{"aud1", "aud2"},
		AudienceCheck: AudienceMatchesHost,
		discoveryURL:  provider.discoveryURL,
	}
	call := func(authHeader string) (*auth.User, error) {
		req := authtest.NewFakeRequestMetadata()
		req.FakeHost = fakeHost
		req.FakeHeader.Set("Authorization", authHeader)
		u, _, err := method.Authenticate(ctx, req)
		return u, err
	}

	Convey("Skipped if no header", t, func() {
		user, err := call("")
		So(err, ShouldBeNil)
		So(user, ShouldBeNil)
	})

	Convey("Skipped if not Bearer", t, func() {
		user, err := call("OAuth zzz")
		So(err, ShouldBeNil)
		So(user, ShouldBeNil)
	})

	Convey("Not JWT token and SkipNonJWT == false", t, func() {
		user, err := call("Bearer " + "im-not-a-jwt")
		So(err, ShouldErrLike, "bad ID token: bad JWT")
		So(user, ShouldBeNil)
	})

	Convey("Not JWT token and SkipNonJWT == true", t, func() {
		method.SkipNonJWT = true

		user, err := call("Bearer " + "im-not-a-jwt")
		So(err, ShouldBeNil)
		So(user, ShouldBeNil)
	})

	Convey("Regular user", t, func() {
		Convey("Happy path", func() {
			user, err := call("Bearer " + provider.mintIDToken(ctx, IDToken{
				Iss:           provider.Issuer,
				EmailVerified: true,
				Sub:           "some-sub",
				Email:         "user@example.com",
				Name:          "Some Dude",
				Picture:       "https://picture/url/s64/photo.jpg",
				Aud:           "some-client-id",
				Iat:           clock.Now(ctx).Unix(),
				Exp:           clock.Now(ctx).Add(time.Hour).Unix(),
			}))
			So(err, ShouldBeNil)
			So(user, ShouldResemble, &auth.User{
				Identity: "user:user@example.com",
				Email:    "user@example.com",
				Name:     "Some Dude",
				Picture:  "https://picture/url/s64/photo.jpg",
				ClientID: "some-client-id",
			})
		})

		Convey("Expired token", func() {
			_, err := call("Bearer " + provider.mintIDToken(ctx, IDToken{
				Iss:           provider.Issuer,
				EmailVerified: true,
				Sub:           "some-sub",
				Email:         "user@example.com",
				Name:          "Some Dude",
				Picture:       "https://picture/url/s64/photo.jpg",
				Aud:           "some-client-id",
				Iat:           clock.Now(ctx).Add(-2 * time.Hour).Unix(),
				Exp:           clock.Now(ctx).Add(-1 * time.Hour).Unix(),
			}))
			So(err, ShouldErrLike, "bad ID token: expired")
		})
	})

	Convey("Service account", t, func() {
		Convey("Happy path using Audience field", func() {
			user, err := call("Bearer " + provider.mintIDToken(ctx, IDToken{
				Iss:           provider.Issuer,
				EmailVerified: true,
				Sub:           "some-sub",
				Email:         "example@example.gserviceaccount.com",
				Aud:           "aud2",
				Iat:           clock.Now(ctx).Unix(),
				Exp:           clock.Now(ctx).Add(time.Hour).Unix(),
			}))
			So(err, ShouldBeNil)
			So(user, ShouldResemble, &auth.User{
				Identity: "user:example@example.gserviceaccount.com",
				Email:    "example@example.gserviceaccount.com",
			})
		})

		Convey("Happy path using AudienceCheck field (direct host hit)", func() {
			user, err := call("Bearer " + provider.mintIDToken(ctx, IDToken{
				Iss:           provider.Issuer,
				EmailVerified: true,
				Sub:           "some-sub",
				Email:         "example@example.gserviceaccount.com",
				Aud:           "https://" + fakeHost,
				Iat:           clock.Now(ctx).Unix(),
				Exp:           clock.Now(ctx).Add(time.Hour).Unix(),
			}))
			So(err, ShouldBeNil)
			So(user, ShouldResemble, &auth.User{
				Identity: "user:example@example.gserviceaccount.com",
				Email:    "example@example.gserviceaccount.com",
			})
		})

		Convey("Happy path using AudienceCheck field (host prefix hit)", func() {
			user, err := call("Bearer " + provider.mintIDToken(ctx, IDToken{
				Iss:           provider.Issuer,
				EmailVerified: true,
				Sub:           "some-sub",
				Email:         "example@example.gserviceaccount.com",
				Aud:           "https://" + fakeHost + "/some/path",
				Iat:           clock.Now(ctx).Unix(),
				Exp:           clock.Now(ctx).Add(time.Hour).Unix(),
			}))
			So(err, ShouldBeNil)
			So(user, ShouldResemble, &auth.User{
				Identity: "user:example@example.gserviceaccount.com",
				Email:    "example@example.gserviceaccount.com",
			})
		})

		Convey("Happy path using OAuth2 client ID", func() {
			user, err := call("Bearer " + provider.mintIDToken(ctx, IDToken{
				Iss:           provider.Issuer,
				EmailVerified: true,
				Sub:           "some-sub",
				Email:         "example@example.gserviceaccount.com",
				Aud:           "blah.apps.googleusercontent.com",
				Iat:           clock.Now(ctx).Unix(),
				Exp:           clock.Now(ctx).Add(time.Hour).Unix(),
			}))
			So(err, ShouldBeNil)
			So(user, ShouldResemble, &auth.User{
				Identity: "user:example@example.gserviceaccount.com",
				Email:    "example@example.gserviceaccount.com",
				ClientID: "blah.apps.googleusercontent.com",
			})
		})

		Convey("Unknown audience", func() {
			_, err := call("Bearer " + provider.mintIDToken(ctx, IDToken{
				Iss:           provider.Issuer,
				EmailVerified: true,
				Sub:           "some-sub",
				Email:         "example@example.gserviceaccount.com",
				Aud:           "what is this",
				Iat:           clock.Now(ctx).Unix(),
				Exp:           clock.Now(ctx).Add(time.Hour).Unix(),
			}))
			So(err, ShouldEqual, auth.ErrBadAudience)
		})
	})
}

type fakeIdentityProvider struct {
	Signer       *signingtest.Signer
	SigningKeyID string
	Issuer       string

	ts           *httptest.Server
	discoveryURL string
}

func (f *fakeIdentityProvider) start() {
	jwks := jwksForTest(f.SigningKeyID, &f.Signer.KeyForTest().PublicKey)

	// Serve the fake discovery document and singing keys.
	f.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discovery":
			w.Write([]byte(fmt.Sprintf(`{
					"issuer": "%s",
					"jwks_uri": "%s/jwks"
				}`, f.Issuer, f.ts.URL)))
		case "/jwks":
			json.NewEncoder(w).Encode(jwks)
		default:
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))

	f.discoveryURL = f.ts.URL + "/discovery"
}

func (f *fakeIdentityProvider) stop() {
	f.ts.Close()
}

func (f *fakeIdentityProvider) mintIDToken(ctx context.Context, tok IDToken) string {
	return idTokenForTest(ctx, &tok, f.SigningKeyID, f.Signer)
}
