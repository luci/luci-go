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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/signing/signingtest"
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

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		original := newToken()
		parsed, err := verifyToken(signToken(original))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, parsed, should.Resemble(original))

		// Alternative issuer form.
		original.Iss = strings.TrimPrefix(issuer, "https://")
		parsed, err = verifyToken(signToken(original))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, parsed, should.Resemble(original))
	})

	ftt.Run("Bad JWT", t, func(t *ftt.Test) {
		_, err := verifyToken("IMANOTAJWT")
		assert.Loosely(t, err, should.ErrLike("bad JWT"))
	})

	ftt.Run("Bad body (not JSON)", t, func(t *ftt.Test) {
		_, err := verifyToken(jwtForTest(ctx, []byte("IAMNOTJSON"), signingKeyID, signer))
		assert.Loosely(t, err, should.ErrLike("can't deserialize JSON"))
	})

	ftt.Run("Bad issuer", t, func(t *ftt.Test) {
		tok := newToken()
		tok.Iss = "something else"
		_, err := verifyToken(signToken(tok))
		assert.Loosely(t, err, should.ErrLike("expecting issuer"))
	})

	ftt.Run("Unverified email", t, func(t *ftt.Test) {
		tok := newToken()
		tok.EmailVerified = false
		_, err := verifyToken(signToken(tok))
		assert.Loosely(t, err, should.ErrLike("is not verified"))
	})

	ftt.Run("No audience", t, func(t *ftt.Test) {
		tok := newToken()
		tok.Aud = ""
		_, err := verifyToken(signToken(tok))
		assert.Loosely(t, err, should.ErrLike("the audience is missing"))
	})

	ftt.Run("No subject", t, func(t *ftt.Test) {
		tok := newToken()
		tok.Sub = ""
		_, err := verifyToken(signToken(tok))
		assert.Loosely(t, err, should.ErrLike("the subject is missing"))
	})

	ftt.Run("Expired", t, func(t *ftt.Test) {
		tok := signToken(newToken())

		// Still good enough after 1h due to clock skew protection.
		tc.Add(time.Hour)
		_, err := verifyToken(tok)
		assert.Loosely(t, err, should.BeNil)

		// Some moments later is considered expired for real.
		tc.Add(allowedClockSkew + time.Second)
		_, err = verifyToken(tok)
		assert.Loosely(t, err, should.ErrLike("expired"))
	})
}
