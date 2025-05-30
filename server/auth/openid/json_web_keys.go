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
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"math/big"

	"go.chromium.org/luci/common/errors"
)

// JSONWebKeySet implements subset of functionality described in RFC7517.
//
// It currently supports only RSA keys and RS256 alg. It's intended to be used
// to represent keys fetched from https://www.googleapis.com/oauth2/v3/certs.
//
// It's used to verify ID token signatures.
type JSONWebKeySet struct {
	keys map[string]rsa.PublicKey // key ID => the key
}

// JSONWebKeySetStruct defines the JSON structure of JSONWebKeySet.
//
// Read it from the wire and pass to NewJSONWebKeySet to get a usable object.
//
// See https://www.iana.org/assignments/jose/jose.xhtml#web-key-parameters.
// We care only about RSA public keys (thus use 'n' and 'e').
type JSONWebKeySetStruct struct {
	Keys []JSONWebKeyStruct `json:"keys"`
}

// JSONWebKeyStruct defines the JSON structure of a single key in the key set.
type JSONWebKeyStruct struct {
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"` // raw URL-safe base64, NOT standard base64
	E   string `json:"e"` // same
}

// NewJSONWebKeySet makes the keyset from raw JSON Web Key set struct.
func NewJSONWebKeySet(parsed *JSONWebKeySetStruct) (*JSONWebKeySet, error) {
	// Pick keys used to verify RS256 signatures.
	keys := make(map[string]rsa.PublicKey, len(parsed.Keys))
	for _, k := range parsed.Keys {
		if k.Kty != "RSA" || k.Alg != "RS256" || k.Use != "sig" {
			continue // not an RSA public key
		}
		if k.Kid == "" {
			// Per spec 'kid' field is optional, but providers we support return them,
			// so make them required to keep the code simpler.
			return nil, errors.New("bad JSON web key: missing 'kid' field")
		}
		pub, err := decodeRSAPublicKey(k.N, k.E)
		if err != nil {
			return nil, errors.Fmt("failed to parse RSA public key in JSON web key: %w", err)
		}
		keys[k.Kid] = pub
	}

	if len(keys) == 0 {
		return nil, errors.New("the JSON web key doc didn't have any signing keys")
	}

	return &JSONWebKeySet{keys: keys}, nil
}

// CheckSignature returns nil if `signed` was indeed signed by the given key.
func (k *JSONWebKeySet) CheckSignature(keyID string, signed, signature []byte) error {
	pub, ok := k.keys[keyID]
	if !ok {
		return errors.Fmt("unknown signing key %q", keyID)
	}
	digest := sha256.Sum256(signed)
	if err := rsa.VerifyPKCS1v15(&pub, crypto.SHA256, digest[:], signature); err != nil {
		return errors.New("bad signature")
	}
	return nil
}

func decodeRSAPublicKey(n, e string) (rsa.PublicKey, error) {
	modulus, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return rsa.PublicKey{}, errors.Fmt("bad modulus encoding: %w", err)
	}
	exp, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return rsa.PublicKey{}, errors.Fmt("bad exponent encoding: %w", err)
	}

	// The exponent should be 4 bytes in BigEndian order. Pad it with zeros if
	// necessary.
	if len(exp) < 4 {
		padded := make([]byte, 4)
		copy(padded[4-len(exp):], exp)
		exp = padded
	}

	return rsa.PublicKey{
		N: (&big.Int{}).SetBytes(modulus),
		E: int(binary.BigEndian.Uint32(exp)),
	}, nil
}
