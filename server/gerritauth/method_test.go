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

package gerritauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func TestAuthMethod(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		const expectedHeader = "X-Gerrit-Auth"
		const expectedAudience = "good-audience"

		assertedUser := AssertedUser{
			AccountID:      12345,
			Emails:         []string{"xyz@example.com", "abc@example.com"},
			PreferredEmail: "abc@example.com",
		}
		assertedChange := AssertedChange{
			Host:         "some-host",
			Repository:   "some/repo",
			ChangeNumber: 99999,
		}

		now := time.Unix(1500000000, 0).UTC()
		ctx, _ := testclock.UseTime(context.Background(), now)

		signer := signingtest.NewSigner(nil)
		certs, _ := signer.Certificates(ctx)
		goodKeyID := signer.KeyNameForTest()

		method := AuthMethod{
			Header:         expectedHeader,
			SignerAccounts: []string{"trusted-issuer"},
			Audience:       expectedAudience,
			testCerts:      certs,
		}

		prepareJWT := func(tok gerritJWT) string {
			bodyBlob, err := json.Marshal(&tok)
			assert.Loosely(t, err, should.BeNil)
			b64hdr := base64.RawURLEncoding.EncodeToString([]byte(
				fmt.Sprintf(`{"alg": "RS256","kid": "%s"}`, goodKeyID),
			))
			b64bdy := base64.RawURLEncoding.EncodeToString(bodyBlob)
			_, sig, err := signer.SignBytes(ctx, []byte(b64hdr+"."+b64bdy))
			assert.Loosely(t, err, should.BeNil)
			return b64hdr + "." + b64bdy + "." + base64.RawURLEncoding.EncodeToString(sig)
		}

		call := func(tok string) (*auth.User, error) {
			req := authtest.NewFakeRequestMetadata()
			req.FakeHeader.Add(expectedHeader, tok)
			user, _, err := method.Authenticate(ctx, req)
			return user, err
		}

		t.Run("Success", func(t *ftt.Test) {
			user, err := call(prepareJWT(gerritJWT{
				Iss:            "trusted-issuer",
				Aud:            expectedAudience,
				Exp:            now.Add(5 * time.Minute).Unix(),
				AssertedUser:   assertedUser,
				AssertedChange: assertedChange,
			}))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, user, should.Resemble(&auth.User{
				Identity: "user:abc@example.com",
				Email:    "abc@example.com",
				Extra: &AssertedInfo{
					User:   assertedUser,
					Change: assertedChange,
				},
			}))
		})

		t.Run("Success, but no preferred email", func(t *ftt.Test) {
			user, err := call(prepareJWT(gerritJWT{
				Iss: "trusted-issuer",
				Aud: expectedAudience,
				Exp: now.Add(5 * time.Minute).Unix(),
				AssertedUser: AssertedUser{
					Emails: []string{"xyz@example.com", "abc@example.com"},
				},
				AssertedChange: assertedChange,
			}))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, user, should.Resemble(&auth.User{
				Identity: "user:xyz@example.com",
				Email:    "xyz@example.com",
				Extra: &AssertedInfo{
					User: AssertedUser{
						Emails: []string{"xyz@example.com", "abc@example.com"},
					},
					Change: assertedChange,
				},
			}))
		})

		t.Run("Unconfigured", func(t *ftt.Test) {
			method.SignerAccounts = nil
			user, err := call("ignored")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, user, should.BeNil)
		})

		t.Run("Missing header", func(t *ftt.Test) {
			method.Header = "Something-Else"
			user, err := call("ignored")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, user, should.BeNil)
		})

		t.Run("Bad token", func(t *ftt.Test) {
			_, err := call("blah.blah.blah")
			assert.Loosely(t, err, should.ErrLike("bad Gerrit JWT"))
		})

		t.Run("Unrecognized issuer", func(t *ftt.Test) {
			_, err := call(prepareJWT(gerritJWT{
				Iss:            "unknown-issuer",
				Aud:            expectedAudience,
				Exp:            now.Add(5 * time.Minute).Unix(),
				AssertedUser:   assertedUser,
				AssertedChange: assertedChange,
			}))
			assert.Loosely(t, err, should.ErrLike("bad Gerrit JWT"))
		})

		t.Run("Bad audience", func(t *ftt.Test) {
			_, err := call(prepareJWT(gerritJWT{
				Iss:            "trusted-issuer",
				Aud:            "wrong-audience",
				Exp:            now.Add(5 * time.Minute).Unix(),
				AssertedUser:   assertedUser,
				AssertedChange: assertedChange,
			}))
			assert.Loosely(t, err, should.ErrLike("bad Gerrit JWT: wrong audience"))
		})

		t.Run("Expired token", func(t *ftt.Test) {
			_, err := call(prepareJWT(gerritJWT{
				Iss:            "trusted-issuer",
				Aud:            expectedAudience,
				Exp:            now.Add(-5 * time.Minute).Unix(),
				AssertedUser:   assertedUser,
				AssertedChange: assertedChange,
			}))
			assert.Loosely(t, err, should.ErrLike("bad Gerrit JWT: expired"))
		})

		t.Run("No emails", func(t *ftt.Test) {
			_, err := call(prepareJWT(gerritJWT{
				Iss:            "trusted-issuer",
				Aud:            expectedAudience,
				Exp:            now.Add(5 * time.Minute).Unix(),
				AssertedChange: assertedChange,
			}))
			assert.Loosely(t, err, should.ErrLike("bad Gerrit JWT: asserted_user.preferred_email and asserted_user.emails are empty"))
		})

		t.Run("Invalid email", func(t *ftt.Test) {
			_, err := call(prepareJWT(gerritJWT{
				Iss: "trusted-issuer",
				Aud: expectedAudience,
				Exp: now.Add(5 * time.Minute).Unix(),
				AssertedUser: AssertedUser{
					PreferredEmail: "this-is-not-an-email",
					Emails: []string{
						"unused@example.com",
					},
				},
				AssertedChange: assertedChange,
			}))
			assert.Loosely(t, err, should.ErrLike("bad Gerrit JWT: unrecognized email format"))
		})
	})
}
