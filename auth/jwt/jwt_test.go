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

package jwt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func TestVerifyJWT(t *testing.T) {
	t.Parallel()

	type fakeBody struct {
		Field string
	}

	ctx := context.Background()
	signer := signingtest.NewSigner(nil)
	certs, _ := signer.Certificates(ctx)
	goodKeyID := signer.KeyNameForTest()

	prepareJWT := func(alg, kid string, body fakeBody) string {
		bodyBlob, err := json.Marshal(&body)
		assert.Loosely(t, err, should.BeNil)
		b64hdr := base64.RawURLEncoding.EncodeToString([]byte(
			fmt.Sprintf(`{"alg": "%s","kid": "%s"}`, alg, kid),
		))
		b64bdy := base64.RawURLEncoding.EncodeToString(bodyBlob)
		_, sig, err := signer.SignBytes(ctx, []byte(b64hdr+"."+b64bdy))
		assert.Loosely(t, err, should.BeNil)
		return b64hdr + "." + b64bdy + "." + base64.RawURLEncoding.EncodeToString(sig)
	}

	verifyJWT := func(tok string) (body fakeBody, err error) {
		err = VerifyAndDecode(tok, &body, certs)
		return
	}

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		body := fakeBody{"body"}
		verifiedBody, err := verifyJWT(prepareJWT("RS256", goodKeyID, body))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, verifiedBody, should.Match(body))
	})

	ftt.Run("Malformed JWT", t, func(t *ftt.Test) {
		_, err := verifyJWT("wat")
		assert.Loosely(t, err, should.ErrLike("expected 3 components"))
		assert.Loosely(t, NotJWT.In(err), should.BeTrue)
	})

	ftt.Run("Bad header format (not b64)", t, func(t *ftt.Test) {
		_, err := verifyJWT("???.aaaa.aaaa")
		assert.Loosely(t, err, should.ErrLike("bad JWT header: not base64"))
		assert.Loosely(t, NotJWT.In(err), should.BeTrue)
	})

	ftt.Run("Bad header format (not json)", t, func(t *ftt.Test) {
		_, err := verifyJWT("aaaa.aaaa.aaaa")
		assert.Loosely(t, err, should.ErrLike("bad JWT header: can't deserialize JSON"))
		assert.Loosely(t, NotJWT.In(err), should.BeTrue)
	})

	ftt.Run("Bad algo", t, func(t *ftt.Test) {
		_, err := verifyJWT(prepareJWT("bad-algo", goodKeyID, fakeBody{"body"}))
		assert.Loosely(t, err, should.ErrLike("only RS256 alg is supported"))
		assert.Loosely(t, NotJWT.In(err), should.BeFalse)
	})

	ftt.Run("Missing key ID", t, func(t *ftt.Test) {
		_, err := verifyJWT(prepareJWT("RS256", "", fakeBody{"body"}))
		assert.Loosely(t, err, should.ErrLike("missing the signing key ID in the header"))
		assert.Loosely(t, NotJWT.In(err), should.BeFalse)
	})

	ftt.Run("Unknown key", t, func(t *ftt.Test) {
		_, err := verifyJWT(prepareJWT("RS256", "unknown-key", fakeBody{"body"}))
		assert.Loosely(t, err, should.ErrLike("no such certificate"))
		assert.Loosely(t, NotJWT.In(err), should.BeFalse)
	})

	ftt.Run("Bad signature encoding", t, func(t *ftt.Test) {
		jwt := prepareJWT("RS256", goodKeyID, fakeBody{"body"})
		_, err := verifyJWT(jwt + "???")
		assert.Loosely(t, err, should.ErrLike("can't base64 decode the signature"))
		assert.Loosely(t, NotJWT.In(err), should.BeFalse)
	})

	ftt.Run("Bad signature", t, func(t *ftt.Test) {
		jwt := prepareJWT("RS256", goodKeyID, fakeBody{"body"})
		_, err := verifyJWT(jwt[:len(jwt)-2])
		assert.Loosely(t, err, should.ErrLike("signature check error"))
		assert.Loosely(t, NotJWT.In(err), should.BeFalse)
	})

	ftt.Run("Bad body JSON", t, func(t *ftt.Test) {
		jwt := prepareJWT("RS256", goodKeyID, fakeBody{"body"})
		var notAStruct int64
		err := VerifyAndDecode(jwt, &notAStruct, certs)
		assert.Loosely(t, err, should.ErrLike("bad body: can't deserialize JSON"))
	})
}
