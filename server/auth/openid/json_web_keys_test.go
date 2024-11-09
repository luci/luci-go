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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func TestParseJSONWebKeySet(t *testing.T) {
	t.Parallel()

	ftt.Run("Happy path", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(keys.keys), should.Equal(1))
		assert.Loosely(t, keys.keys["key-1"].E, should.Equal(65537))
	})

	ftt.Run("No key ID", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.ErrLike("missing 'kid' field"))
	})

	ftt.Run("Bad e", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.ErrLike("bad exponent encoding"))
	})

	ftt.Run("Bad n", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.ErrLike("bad modulus encoding"))
	})

	ftt.Run("No signing keys", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.ErrLike("didn't have any signing keys"))
	})
}

func TestCheckSignature(t *testing.T) {
	t.Parallel()

	ftt.Run("With signed blob", t, func(t *ftt.Test) {
		ctx := context.Background()
		signer := signingtest.NewSigner(nil)
		keys := JSONWebKeySet{
			keys: map[string]rsa.PublicKey{
				"key-1": signer.KeyForTest().PublicKey,
			},
		}

		var blob = []byte("blah blah")
		_, sig, err := signer.SignBytes(ctx, blob)
		assert.Loosely(t, err, should.BeNil)

		t.Run("Good signature", func(t *ftt.Test) {
			assert.Loosely(t, keys.CheckSignature("key-1", blob, sig), should.BeNil)
		})

		t.Run("Bad signature", func(t *ftt.Test) {
			assert.Loosely(t, keys.CheckSignature("key-1", []byte("something else"), sig), should.ErrLike("bad signature"))
		})

		t.Run("Unknown key", func(t *ftt.Test) {
			assert.Loosely(t, keys.CheckSignature("unknown", blob, sig), should.ErrLike("unknown signing key"))
		})
	})
}
