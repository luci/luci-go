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

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAuthMethod(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
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
			So(err, ShouldBeNil)
			b64hdr := base64.RawURLEncoding.EncodeToString([]byte(
				fmt.Sprintf(`{"alg": "RS256","kid": "%s"}`, goodKeyID),
			))
			b64bdy := base64.RawURLEncoding.EncodeToString(bodyBlob)
			_, sig, err := signer.SignBytes(ctx, []byte(b64hdr+"."+b64bdy))
			So(err, ShouldBeNil)
			return b64hdr + "." + b64bdy + "." + base64.RawURLEncoding.EncodeToString(sig)
		}

		call := func(tok string) (*auth.User, error) {
			req := authtest.NewFakeRequestMetadata()
			req.FakeHeader.Add(expectedHeader, tok)
			user, _, err := method.Authenticate(ctx, req)
			return user, err
		}

		Convey("Success", func() {
			user, err := call(prepareJWT(gerritJWT{
				Iss:            "trusted-issuer",
				Aud:            expectedAudience,
				Exp:            now.Add(5 * time.Minute).Unix(),
				AssertedUser:   assertedUser,
				AssertedChange: assertedChange,
			}))
			So(err, ShouldBeNil)
			So(user, ShouldResemble, &auth.User{
				Identity: "user:abc@example.com",
				Email:    "abc@example.com",
				Extra: &AssertedInfo{
					User:   assertedUser,
					Change: assertedChange,
				},
			})
		})

		Convey("Success, but no preferred email", func() {
			user, err := call(prepareJWT(gerritJWT{
				Iss: "trusted-issuer",
				Aud: expectedAudience,
				Exp: now.Add(5 * time.Minute).Unix(),
				AssertedUser: AssertedUser{
					Emails: []string{"xyz@example.com", "abc@example.com"},
				},
				AssertedChange: assertedChange,
			}))
			So(err, ShouldBeNil)
			So(user, ShouldResemble, &auth.User{
				Identity: "user:xyz@example.com",
				Email:    "xyz@example.com",
				Extra: &AssertedInfo{
					User: AssertedUser{
						Emails: []string{"xyz@example.com", "abc@example.com"},
					},
					Change: assertedChange,
				},
			})
		})

		Convey("Unconfigured", func() {
			method.SignerAccounts = nil
			user, err := call("ignored")
			So(err, ShouldBeNil)
			So(user, ShouldBeNil)
		})

		Convey("Missing header", func() {
			method.Header = "Something-Else"
			user, err := call("ignored")
			So(err, ShouldBeNil)
			So(user, ShouldBeNil)
		})

		Convey("Bad token", func() {
			_, err := call("blah.blah.blah")
			So(err, ShouldErrLike, "bad Gerrit JWT")
		})

		Convey("Unrecognized issuer", func() {
			_, err := call(prepareJWT(gerritJWT{
				Iss:            "unknown-issuer",
				Aud:            expectedAudience,
				Exp:            now.Add(5 * time.Minute).Unix(),
				AssertedUser:   assertedUser,
				AssertedChange: assertedChange,
			}))
			So(err, ShouldErrLike, "bad Gerrit JWT")
		})

		Convey("Bad audience", func() {
			_, err := call(prepareJWT(gerritJWT{
				Iss:            "trusted-issuer",
				Aud:            "wrong-audience",
				Exp:            now.Add(5 * time.Minute).Unix(),
				AssertedUser:   assertedUser,
				AssertedChange: assertedChange,
			}))
			So(err, ShouldErrLike, "bad Gerrit JWT: wrong audience")
		})

		Convey("Expired token", func() {
			_, err := call(prepareJWT(gerritJWT{
				Iss:            "trusted-issuer",
				Aud:            expectedAudience,
				Exp:            now.Add(-5 * time.Minute).Unix(),
				AssertedUser:   assertedUser,
				AssertedChange: assertedChange,
			}))
			So(err, ShouldErrLike, "bad Gerrit JWT: expired")
		})

		Convey("No emails", func() {
			_, err := call(prepareJWT(gerritJWT{
				Iss:            "trusted-issuer",
				Aud:            expectedAudience,
				Exp:            now.Add(5 * time.Minute).Unix(),
				AssertedChange: assertedChange,
			}))
			So(err, ShouldErrLike, "bad Gerrit JWT: asserted_user.preferred_email and asserted_user.emails are empty")
		})

		Convey("Invalid email", func() {
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
			So(err, ShouldErrLike, "bad Gerrit JWT: unrecognized email format")
		})
	})
}
