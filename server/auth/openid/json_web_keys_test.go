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
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"testing"

	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseJSONWebKeySet(t *testing.T) {
	t.Parallel()

	Convey("Happy path", t, func() {
		keys, err := NewJSONWebKeySet(&JSONWebKeySetStruct{
			Keys: []JSONWebKeyStruct{
				{
					Kty: "RSA",
					Alg: "RS256",
					Use: "sig",
					Kid: "key-1",
					N:   "sJ46htmsgvo99FyibDKBh6zHGr2hldxfHt-6y9VTrDo3jfuFeA8xFG3Lb9LkJE4qlCvC9QRv3S0TWhQcTg9uGN2mY7B7ccIRaD2AL2hh87mFAkQzoRPxg5lvS6QisgiFQcH-oea-4iBYlBHyMemoXR8pRSwTeQKxzvZ2zohl5c-yE77vhrmDV9_n3viJH1YBYJk2weAj9Pqcfj7_cLF6tR1vd4voTL29WrWJwkO2KG6FcXrc0DQQba8nXPc-TFJd1Z9aSyzxS91rxnj8wnMbPdXPfG2jryT1Dg_LhWtDjIG7m2ZVN-_wL1hHhxC6m7yl4jSIaAO5rjKMToq5wrlgVQ",
					E:   "AQAB",
				},
				{
					Kty: "SOMETHING-ELSE",
				},
			},
		})
		So(err, ShouldBeNil)
		So(len(keys.keys), ShouldEqual, 1)
		So(keys.keys["key-1"].E, ShouldEqual, 65537)
	})

	Convey("No key ID", t, func() {
		_, err := NewJSONWebKeySet(&JSONWebKeySetStruct{
			Keys: []JSONWebKeyStruct{
				{
					Kty: "RSA",
					Alg: "RS256",
					Use: "sig",
					N:   "sJ46htmsgvo99FyibD",
					E:   "AQAB",
				},
			},
		})
		So(err, ShouldErrLike, "missing 'kid' field")
	})

	Convey("Bad e", t, func() {
		_, err := NewJSONWebKeySet(&JSONWebKeySetStruct{
			Keys: []JSONWebKeyStruct{
				{
					Kty: "RSA",
					Alg: "RS256",
					Use: "sig",
					Kid: "key-1",
					N:   "sJ46htmsgvo99FyibD",
					E:   "????",
				},
			},
		})
		So(err, ShouldErrLike, "bad exponent encoding")
	})

	Convey("Bad n", t, func() {
		_, err := NewJSONWebKeySet(&JSONWebKeySetStruct{
			Keys: []JSONWebKeyStruct{
				{
					Kty: "RSA",
					Alg: "RS256",
					Use: "sig",
					Kid: "key-1",
					N:   "????",
					E:   "AQAB",
				},
			},
		})
		So(err, ShouldErrLike, "bad modulus encoding")
	})

	Convey("No signing keys", t, func() {
		_, err := NewJSONWebKeySet(&JSONWebKeySetStruct{
			Keys: []JSONWebKeyStruct{
				{
					Kty: "RSA",
					Alg: "RS256",
					Use: "encrypt",
					Kid: "key-1",
					N:   "sJ46htmsgvo99FyibD",
					E:   "AQAB",
				},
			},
		})
		So(err, ShouldErrLike, "didn't have any signing keys")
	})
}

func TestVerifyJWT(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	signer := signingtest.NewSigner(nil)
	keys := JSONWebKeySet{
		keys: map[string]rsa.PublicKey{
			"key-1": signer.KeyForTest().PublicKey,
		},
	}

	prepareJWT := func(alg, kid string, body []byte) string {
		b64hdr := base64.RawURLEncoding.EncodeToString([]byte(
			fmt.Sprintf(`{"alg": "%s","kid": "%s"}`, alg, kid)))
		b64bdy := base64.RawURLEncoding.EncodeToString(body)
		_, sig, err := signer.SignBytes(ctx, []byte(b64hdr+"."+b64bdy))
		So(err, ShouldBeNil)
		return b64hdr + "." + b64bdy + "." + base64.RawURLEncoding.EncodeToString(sig)
	}

	Convey("Happy path", t, func() {
		body := []byte(`blah blah blah`)
		verifiedBody, err := keys.VerifyJWT(prepareJWT("RS256", "key-1", body))
		So(err, ShouldBeNil)
		So(verifiedBody, ShouldResemble, body)
	})

	Convey("Malformed JWT", t, func() {
		_, err := keys.VerifyJWT("wat")
		So(err, ShouldErrLike, "expected 3 components")
	})

	Convey("Bad header format (not b64)", t, func() {
		_, err := keys.VerifyJWT("???.aaaa.aaaa")
		So(err, ShouldErrLike, "bad JWT header - not base64")
	})

	Convey("Bad header format (not json)", t, func() {
		_, err := keys.VerifyJWT("aaaa.aaaa.aaaa")
		So(err, ShouldErrLike, "bad JWT header - not JSON")
	})

	Convey("Bad algo", t, func() {
		_, err := keys.VerifyJWT(prepareJWT("bad-algo", "key-1", []byte("body")))
		So(err, ShouldErrLike, "only RS256 alg is supported")
	})

	Convey("Missing key ID", t, func() {
		_, err := keys.VerifyJWT(prepareJWT("RS256", "", []byte("body")))
		So(err, ShouldErrLike, "missing the signing key ID in the header")
	})

	Convey("Unknown key", t, func() {
		_, err := keys.VerifyJWT(prepareJWT("RS256", "unknown-key", []byte("body")))
		So(err, ShouldErrLike, "unknown signing key")
	})

	Convey("Bad signature encoding", t, func() {
		jwt := prepareJWT("RS256", "key-1", []byte("body"))
		_, err := keys.VerifyJWT(jwt + "???")
		So(err, ShouldErrLike, "can't base64 decode the signature")
	})

	Convey("Bad signature", t, func() {
		jwt := prepareJWT("RS256", "key-1", []byte("body"))
		_, err := keys.VerifyJWT(jwt[:len(jwt)-2])
		So(err, ShouldErrLike, "bad signature")
	})
}
