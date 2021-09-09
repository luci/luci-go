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

func TestCheckSignature(t *testing.T) {
	t.Parallel()

	Convey("With signed blob", t, func() {
		ctx := context.Background()
		signer := signingtest.NewSigner(nil)
		keys := JSONWebKeySet{
			keys: map[string]rsa.PublicKey{
				"key-1": signer.KeyForTest().PublicKey,
			},
		}

		var blob = []byte("blah blah")
		_, sig, err := signer.SignBytes(ctx, blob)
		So(err, ShouldBeNil)

		Convey("Good signature", func() {
			So(keys.CheckSignature("key-1", blob, sig), ShouldBeNil)
		})

		Convey("Bad signature", func() {
			So(keys.CheckSignature("key-1", []byte("something else"), sig), ShouldErrLike, "bad signature")
		})

		Convey("Unknown key", func() {
			So(keys.CheckSignature("unknown", blob, sig), ShouldErrLike, "unknown signing key")
		})
	})
}
