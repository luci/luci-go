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
	"encoding/json"
	"math/big"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// NotJWT is an error tag used to indicate that the string passed to VerifyJWT
// is not in fact structurally a JWT.
var NotJWT = errors.BoolTag{Key: errors.NewTagKey("not a JSON web token")}

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
			return nil, errors.Reason("bad JSON web key: missing 'kid' field").Err()
		}
		pub, err := decodeRSAPublicKey(k.N, k.E)
		if err != nil {
			return nil, errors.Annotate(err, "failed to parse RSA public key in JSON web key").Err()
		}
		keys[k.Kid] = pub
	}

	if len(keys) == 0 {
		return nil, errors.Reason("the JSON web key doc didn't have any signing keys").Err()
	}

	return &JSONWebKeySet{keys: keys}, nil
}

// VerifyJWT checks JWT signature and returns the token body.
//
// Supports only non-encrypted RS256-signed JWTs. See RFC7519.
func (k *JSONWebKeySet) VerifyJWT(jwt string) (body []byte, err error) {
	chunks := strings.Split(jwt, ".")
	if len(chunks) != 3 {
		return nil, errors.Reason("bad JWT: expected 3 components separated by '.'").Tag(NotJWT).Err()
	}

	// Check the header, grab the corresponding public key.
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := unmarshalB64JSON(chunks[0], &hdr); err != nil {
		return nil, errors.Annotate(err, "bad JWT header").Tag(NotJWT).Err()
	}
	if hdr.Alg != "RS256" {
		return nil, errors.Reason("bad JWT: only RS256 alg is supported, not %q", hdr.Alg).Err()
	}
	if hdr.Kid == "" {
		return nil, errors.Reason("bad JWT: missing the signing key ID in the header").Err()
	}
	pub, ok := k.keys[hdr.Kid]
	if !ok {
		return nil, errors.Reason("can't verify JWT: unknown signing key %q", hdr.Kid).Err()
	}

	// Decode the signature.
	sig, err := base64.RawURLEncoding.DecodeString(chunks[2])
	if err != nil {
		return nil, errors.Annotate(err, "bad JWT: can't base64 decode the signature").Err()
	}

	// Check the signature. The signed string is "b64(header).b64(body)".
	hasher := sha256.New()
	hasher.Write([]byte(chunks[0]))
	hasher.Write([]byte{'.'})
	hasher.Write([]byte(chunks[1]))
	if err := rsa.VerifyPKCS1v15(&pub, crypto.SHA256, hasher.Sum(nil), sig); err != nil {
		return nil, errors.Reason("bad JWT: bad signature").Err()
	}

	// Decode the body. There should be no errors here generally, the encoded body
	// is signed and the signature was already verified.
	body, err = base64.RawURLEncoding.DecodeString(chunks[1])
	if err != nil {
		return nil, errors.Annotate(err, "bad JWT: can't base64 decode the body").Err()
	}
	return body, nil
}

////////////////////////////////////////////////////////////////////////////////

func unmarshalB64JSON(blob string, out interface{}) error {
	raw, err := base64.RawURLEncoding.DecodeString(blob)
	if err != nil {
		return errors.Annotate(err, "not base64").Err()
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.Annotate(err, "not JSON").Err()
	}
	return nil
}

func decodeRSAPublicKey(n, e string) (rsa.PublicKey, error) {
	modulus, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return rsa.PublicKey{}, errors.Annotate(err, "bad modulus encoding").Err()
	}
	exp, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return rsa.PublicKey{}, errors.Annotate(err, "bad exponent encoding").Err()
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
