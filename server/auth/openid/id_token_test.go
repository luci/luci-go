// Copyright 2017 The LUCI Authors.
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
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestVerifyIDToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, tc := testclock.UseTime(ctx, time.Unix(1442540000, 0))

	// Prepare the signing keys.
	const issuer = "https://issuer.example.com"
	const signingKeyID = "signing-key"

	signer := signingtest.NewSigner(nil)
	jwks, _ := NewJSONWebKeySet(jwksForTest(signingKeyID, &signer.KeyForTest().PublicKey))

	newToken := func() *IDToken {
		return &IDToken{
			Iss:           issuer,
			EmailVerified: true,
			Sub:           "user_id_sub",
			Email:         "user@example.com",
			Name:          "Some Dude",
			Picture:       "https://picture/url/s64/photo.jpg",
			Aud:           "client_id",
			Iat:           clock.Now(ctx).Unix(),
			Exp:           clock.Now(ctx).Add(time.Hour).Unix(),
		}
	}
	signToken := func(t *IDToken) string {
		return idTokenForTest(ctx, t, signingKeyID, signer)
	}
	verifyToken := func(token string) (*IDToken, error) {
		return VerifyIDToken(ctx, token, jwks, issuer)
	}

	Convey("Happy path", t, func() {
		original := newToken()
		parsed, err := verifyToken(signToken(original))
		So(err, ShouldBeNil)
		So(parsed, ShouldResemble, original)

		// Alternative issuer form.
		original.Iss = strings.TrimPrefix(issuer, "https://")
		parsed, err = verifyToken(signToken(original))
		So(err, ShouldBeNil)
		So(parsed, ShouldResemble, original)
	})

	Convey("Bad JWT", t, func() {
		_, err := verifyToken("IMANOTAJWT")
		So(err, ShouldErrLike, "bad JWT")
	})

	Convey("Bad body (not JSON)", t, func() {
		_, err := verifyToken(jwtForTest(ctx, []byte("IAMNOTJSON"), signingKeyID, signer))
		So(err, ShouldErrLike, "can't deserialize JSON")
	})

	Convey("Bad issuer", t, func() {
		tok := newToken()
		tok.Iss = "something else"
		_, err := verifyToken(signToken(tok))
		So(err, ShouldErrLike, "expecting issuer")
	})

	Convey("Unverified email", t, func() {
		tok := newToken()
		tok.EmailVerified = false
		_, err := verifyToken(signToken(tok))
		So(err, ShouldErrLike, "is not verified")
	})

	Convey("No audience", t, func() {
		tok := newToken()
		tok.Aud = ""
		_, err := verifyToken(signToken(tok))
		So(err, ShouldErrLike, "the audience is missing")
	})

	Convey("No subject", t, func() {
		tok := newToken()
		tok.Sub = ""
		_, err := verifyToken(signToken(tok))
		So(err, ShouldErrLike, "the subject is missing")
	})

	Convey("Expired", t, func() {
		tok := signToken(newToken())

		// Still good enough after 1h due to clock skew protection.
		tc.Add(time.Hour)
		_, err := verifyToken(tok)
		So(err, ShouldBeNil)

		// Some moments later is considered expired for real.
		tc.Add(allowedClockSkew + time.Second)
		_, err = verifyToken(tok)
		So(err, ShouldErrLike, "expired")
	})
}
