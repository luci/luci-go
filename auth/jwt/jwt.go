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

// Package jwt contains low-level utilities for verifying JSON Web Tokens.
//
// Supports only non-encrypted RS256 tokens with `kid` header field populated,
// as produced by Google Cloud Platform.
package jwt

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
)

// NotJWT is an error tag used to indicate that the string passed to
// VerifyAndDecode is not in fact structurally a JWT.
var NotJWT = errtag.Make("not a JSON web token", true)

// SignatureVerifier can verify RS256 signatures.
type SignatureVerifier interface {
	// CheckSignature returns nil if `signed` was indeed signed by given key.
	CheckSignature(keyID string, signed, signature []byte) error
}

// UnsafeDecode extracts the payload of a JWT **without verifying it**.
//
// It must always be followed by VerifyAndDecode. Useful to "peek" inside the
// token to see who it was supposedly signed by.
func UnsafeDecode(jwt string, dest any) error {
	chunks := strings.Split(jwt, ".")
	if len(chunks) != 3 {
		return errors.Reason("bad JWT: expected 3 components separated by '.'").Tag(NotJWT).Err()
	}
	if err := unmarshalB64JSON(chunks[1], dest); err != nil {
		return errors.Annotate(err, "bad JWT: bad body").Err()
	}
	return nil
}

// VerifyAndDecode deconstructs the token, verifies its signature using the
// given `verifier` and on success deserializes its body into `dest`.
//
// Returns errors tagged with NotJWT if `token` doesn't look like a JWT at
// all. Other errors (like signature verification check errors) are returned
// without this tag.
//
// Doesn't interpret any JWT claims in the body, just deserializes them into
// `dest`. The caller is responsible for checking them.
func VerifyAndDecode(jwt string, dest any, verifier SignatureVerifier) error {
	chunks := strings.Split(jwt, ".")
	if len(chunks) != 3 {
		return errors.Reason("bad JWT: expected 3 components separated by '.'").Tag(NotJWT).Err()
	}

	// Check we've got the supported kind of token.
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := unmarshalB64JSON(chunks[0], &hdr); err != nil {
		return errors.Annotate(err, "bad JWT header").Tag(NotJWT).Err()
	}
	if hdr.Alg != "RS256" {
		return errors.Reason("bad JWT: only RS256 alg is supported, not %q", hdr.Alg).Err()
	}
	if hdr.Kid == "" {
		return errors.Reason("bad JWT: missing the signing key ID in the header").Err()
	}

	// Decode the signature.
	sig, err := base64.RawURLEncoding.DecodeString(chunks[2])
	if err != nil {
		return errors.Annotate(err, "bad JWT: can't base64 decode the signature").Err()
	}

	// Check the signature. The signed string is "b64(header).b64(body)".
	signed := chunks[0] + "." + chunks[1]
	if err := verifier.CheckSignature(hdr.Kid, []byte(signed), sig); err != nil {
		return errors.Annotate(err, "bad JWT: signature check error").Err()
	}

	// Decode and deserialize the body. There should be no errors here generally,
	// the encoded body is signed and the signature was already verified.
	if err := unmarshalB64JSON(chunks[1], dest); err != nil {
		return errors.Annotate(err, "bad JWT: bad body").Err()
	}
	return nil
}

func unmarshalB64JSON(blob string, out any) error {
	raw, err := base64.RawURLEncoding.DecodeString(blob)
	if err != nil {
		return errors.Annotate(err, "not base64").Err()
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.Annotate(err, "can't deserialize JSON").Err()
	}
	return nil
}
