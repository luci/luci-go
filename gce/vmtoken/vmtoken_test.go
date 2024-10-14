// Copyright 2019 The LUCI Authors.
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

package vmtoken

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMatches(t *testing.T) {
	t.Parallel()

	ftt.Run("Matches", t, func(t *ftt.Test) {
		t.Run("no payload", func(t *ftt.Test) {
			c := context.Background()
			assert.Loosely(t, Matches(c, "instance", "zone", "project"), should.BeFalse)
		})

		t.Run("payload", func(t *ftt.Test) {
			c := withPayload(context.Background(), &Payload{
				Instance: "instance",
				Project:  "project",
				Zone:     "zone",
			})

			t.Run("mismatch", func(t *ftt.Test) {
				assert.Loosely(t, Matches(c, "mismatch", "zone", "project"), should.BeFalse)
				assert.Loosely(t, Matches(c, "instance", "mismatch", "project"), should.BeFalse)
				assert.Loosely(t, Matches(c, "instance", "zone", "mismatch"), should.BeFalse)
			})

			t.Run("match", func(t *ftt.Test) {
				assert.Loosely(t, Matches(c, "instance", "zone", "project"), should.BeTrue)
			})
		})
	})
}

func TestVerify(t *testing.T) {
	t.Parallel()

	// This is a real token produced by GCE, without the signature.
	const realTokenUnsigned = `eyJhbGciOiJSUzI1NiIsImtpZCI6IjA5MDVkNmY5Y2Q5YjBmMWY4NTJl` +
		`OGIyMDdlOGY2NzNhYmNhNGJmNzUiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOiJodHRwczovL2V4Y` +
		`W1wbGUuY29tIiwiYXpwIjoiMTE1NjE1Njc0NzIzMTA1NTU1Nzg4IiwiZW1haWwiOiJjaHJvb` +
		`WUtc3dhcm1pbmdAY2hyb21lY29tcHV0ZS5nb29nbGUuY29tLmlhbS5nc2VydmljZWFjY291b` +
		`nQuY29tIiwiZW1haWxfdmVyaWZpZWQiOnRydWUsImV4cCI6MTU1MzU2ODAzMiwiZ29vZ2xlI` +
		`jp7ImNvbXB1dGVfZW5naW5lIjp7Imluc3RhbmNlX2NyZWF0aW9uX3RpbWVzdGFtcCI6MTUzO` +
		`TM4OTQzNSwiaW5zdGFuY2VfaWQiOiI3NDI1MzUwMzU1MDI1NzM0NTUwIiwiaW5zdGFuY2Vfb` +
		`mFtZSI6InN3YXJtNC1jNyIsInByb2plY3RfaWQiOiJnb29nbGUuY29tOmNocm9tZWNvbXB1d` +
		`GUiLCJwcm9qZWN0X251bWJlciI6MTgyNjE1NTA2OTc5LCJ6b25lIjoidXMtY2VudHJhbDEtY` +
		`iJ9fSwiaWF0IjoxNTUzNTY0NDMyLCJpc3MiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb` +
		`20iLCJzdWIiOiIxMTU2MTU2NzQ3MjMxMDU1NTU3ODgifQ`

	// "Real" token, except the signature is mocked.
	var realToken = realTokenUnsigned + "." + b64("REDACTED_SIGNATURE")

	// Parameters encoded in the token above.
	const (
		testKeyID    = "0905d6f9cd9b0f1f852e8b207e8f673abca4bf75"
		testIat      = 1553564432
		testExp      = testIat + 3600
		testProject  = "google.com:chromecompute"
		testZone     = "us-central1-b"
		testInstance = "swarm4-c7"
		testAudience = "https://example.com"
	)

	ftt.Run("With mocked certs and time", t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), time.Unix(testIat, 0))
		certs := mockedCerts{}

		t.Run("Decode real token", func(t *ftt.Test) {
			payload, err := verifyImpl(ctx, realToken, &certs)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, payload, should.Resemble(&Payload{
				Project:  testProject,
				Zone:     testZone,
				Instance: testInstance,
				Audience: testAudience,
			}))
			assert.Loosely(t, certs.calls, should.HaveLength(1))
			assert.Loosely(t, certs.calls[0], should.Resemble(checkSignatureCall{
				key:       testKeyID,
				signed:    []byte(realTokenUnsigned),
				signature: []byte("REDACTED_SIGNATURE"),
			}))
		})

		t.Run("Token used too soon", func(t *ftt.Test) {
			ctx, _ = testclock.UseTime(context.Background(), time.Unix(testIat-60, 0))
			_, err := verifyImpl(ctx, realToken, &certs)
			assert.Loosely(t, err, should.ErrLike("bad JWT: too early (now 1553564372 < iat 1553564432)"))
		})

		t.Run("Token used too late", func(t *ftt.Test) {
			ctx, _ = testclock.UseTime(context.Background(), time.Unix(testExp+60, 0))
			_, err := verifyImpl(ctx, realToken, &certs)
			assert.Loosely(t, err, should.ErrLike("bad JWT: expired (now 1553568092 > exp 1553568032)"))
		})

		t.Run("Bad JWT structure", func(t *ftt.Test) {
			_, err := verifyImpl(ctx, realTokenUnsigned, &certs) // no signature part
			assert.Loosely(t, err, should.ErrLike("expected 3 components"))
		})

		t.Run("Not base64 header", func(t *ftt.Test) {
			_, err := verifyImpl(ctx, "!!!!.AAAA.AAAA", &certs)
			assert.Loosely(t, err, should.ErrLike("bad JWT header: not base64"))
		})

		t.Run("Not JSON header", func(t *ftt.Test) {
			_, err := verifyImpl(ctx, b64("huh")+".AAAA.AAAA", &certs)
			assert.Loosely(t, err, should.ErrLike("bad JWT header: not JSON"))
		})

		t.Run("Wrong algo", func(t *ftt.Test) {
			_, err := verifyImpl(ctx, b64(`{"alg":"huh"}`)+".AAAA.AAAA", &certs)
			assert.Loosely(t, err, should.ErrLike(`bad JWT: only RS256 alg is supported, not "huh"`))
		})

		t.Run("Missing key ID", func(t *ftt.Test) {
			_, err := verifyImpl(ctx, b64(`{"alg":"RS256"}`)+".AAAA.AAAA", &certs)
			assert.Loosely(t, err, should.ErrLike(`bad JWT: missing the signing key ID in the header`))
		})

		t.Run("Bad base64 signature", func(t *ftt.Test) {
			_, err := verifyImpl(ctx, hdr()+".AAAA.!!!!", &certs)
			assert.Loosely(t, err, should.ErrLike("bad JWT: can't base64 decode the signature"))
		})

		t.Run("Signature check error", func(t *ftt.Test) {
			certs.err = fmt.Errorf("boom")
			_, err := verifyImpl(ctx, hdr()+".AAAA."+b64("sig"), &certs)
			assert.Loosely(t, err, should.ErrLike("bad JWT: bad signature: boom"))
		})

		t.Run("Bad payload", func(t *ftt.Test) {
			_, err := verifyImpl(ctx, hdr()+".!!!!."+b64("sig"), &certs)
			assert.Loosely(t, err, should.ErrLike("bad JWT payload: not base64"))
		})

		t.Run("Missing `google.compute_engine` section", func(t *ftt.Test) {
			_, err := verifyImpl(ctx, hdr()+"."+b64(`{}`)+"."+b64("sig"), &certs)
			assert.Loosely(t, err, should.ErrLike("no google.compute_engine in the GCE VM token, use 'full' format"))
		})
	})
}

func hdr() string {
	return b64(`{"alg":"RS256","kid":"key id"}`)
}

func b64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

type mockedCerts struct {
	calls []checkSignatureCall
	err   error
}

type checkSignatureCall struct {
	key       string
	signed    []byte
	signature []byte
}

func (m *mockedCerts) CheckSignature(key string, signed, signature []byte) error {
	m.calls = append(m.calls, checkSignatureCall{key, signed, signature})
	return m.err
}
