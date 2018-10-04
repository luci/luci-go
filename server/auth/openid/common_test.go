// Copyright 2015 The LUCI Authors.
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
	"encoding/binary"
	"encoding/json"
	"fmt"

	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func jwksForTest(keyID string, pubKey *rsa.PublicKey) *JSONWebKeySetStruct {
	modulus := pubKey.N.Bytes()
	exp := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(exp, uint32(pubKey.E))
	return &JSONWebKeySetStruct{
		Keys: []JSONWebKeyStruct{
			{
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
				Kid: keyID,
				N:   base64.RawURLEncoding.EncodeToString(modulus),
				E:   base64.RawURLEncoding.EncodeToString(exp),
			},
		},
	}
}

func jwtForTest(ctx context.Context, body []byte, keyID string, signer *signingtest.Signer) string {
	b64hdr := base64.RawURLEncoding.EncodeToString([]byte(
		fmt.Sprintf(`{"alg": "RS256","kid": "%s"}`, keyID)))
	b64bdy := base64.RawURLEncoding.EncodeToString(body)
	_, sig, err := signer.SignBytes(ctx, []byte(b64hdr+"."+b64bdy))
	if err != nil {
		panic(err)
	}
	return b64hdr + "." + b64bdy + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func idTokenForTest(ctx context.Context, tok *IDToken, keyID string, signer *signingtest.Signer) string {
	body, err := json.Marshal(tok)
	if err != nil {
		panic(err)
	}
	return jwtForTest(ctx, body, keyID, signer)
}
