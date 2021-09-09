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

	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
		So(err, ShouldBeNil)
		b64hdr := base64.RawURLEncoding.EncodeToString([]byte(
			fmt.Sprintf(`{"alg": "%s","kid": "%s"}`, alg, kid),
		))
		b64bdy := base64.RawURLEncoding.EncodeToString(bodyBlob)
		_, sig, err := signer.SignBytes(ctx, []byte(b64hdr+"."+b64bdy))
		So(err, ShouldBeNil)
		return b64hdr + "." + b64bdy + "." + base64.RawURLEncoding.EncodeToString(sig)
	}

	verifyJWT := func(tok string) (body fakeBody, err error) {
		err = VerifyAndDecode(tok, &body, certs)
		return
	}

	Convey("Happy path", t, func() {
		body := fakeBody{"body"}
		verifiedBody, err := verifyJWT(prepareJWT("RS256", goodKeyID, body))
		So(err, ShouldBeNil)
		So(verifiedBody, ShouldResemble, body)
	})

	Convey("Malformed JWT", t, func() {
		_, err := verifyJWT("wat")
		So(err, ShouldErrLike, "expected 3 components")
		So(NotJWT.In(err), ShouldBeTrue)
	})

	Convey("Bad header format (not b64)", t, func() {
		_, err := verifyJWT("???.aaaa.aaaa")
		So(err, ShouldErrLike, "bad JWT header: not base64")
		So(NotJWT.In(err), ShouldBeTrue)
	})

	Convey("Bad header format (not json)", t, func() {
		_, err := verifyJWT("aaaa.aaaa.aaaa")
		So(err, ShouldErrLike, "bad JWT header: can't deserialize JSON")
		So(NotJWT.In(err), ShouldBeTrue)
	})

	Convey("Bad algo", t, func() {
		_, err := verifyJWT(prepareJWT("bad-algo", goodKeyID, fakeBody{"body"}))
		So(err, ShouldErrLike, "only RS256 alg is supported")
		So(NotJWT.In(err), ShouldBeFalse)
	})

	Convey("Missing key ID", t, func() {
		_, err := verifyJWT(prepareJWT("RS256", "", fakeBody{"body"}))
		So(err, ShouldErrLike, "missing the signing key ID in the header")
		So(NotJWT.In(err), ShouldBeFalse)
	})

	Convey("Unknown key", t, func() {
		_, err := verifyJWT(prepareJWT("RS256", "unknown-key", fakeBody{"body"}))
		So(err, ShouldErrLike, "no such certificate")
		So(NotJWT.In(err), ShouldBeFalse)
	})

	Convey("Bad signature encoding", t, func() {
		jwt := prepareJWT("RS256", goodKeyID, fakeBody{"body"})
		_, err := verifyJWT(jwt + "???")
		So(err, ShouldErrLike, "can't base64 decode the signature")
		So(NotJWT.In(err), ShouldBeFalse)
	})

	Convey("Bad signature", t, func() {
		jwt := prepareJWT("RS256", goodKeyID, fakeBody{"body"})
		_, err := verifyJWT(jwt[:len(jwt)-2])
		So(err, ShouldErrLike, "signature check error")
		So(NotJWT.In(err), ShouldBeFalse)
	})

	Convey("Bad body JSON", t, func() {
		jwt := prepareJWT("RS256", goodKeyID, fakeBody{"body"})
		var notAStruct int64
		err := VerifyAndDecode(jwt, &notAStruct, certs)
		So(err, ShouldErrLike, "bad body: can't deserialize JSON")
	})
}
