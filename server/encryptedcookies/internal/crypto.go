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

package internal

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/google/tink/go/tink"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/encryptedcookies/internal/encryptedcookiespb"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"
)

const (
	aeadContextState         = "encryptedcookies:state"
	aeadContextPrivate       = "encryptedcookies:private"
	aeadContextSessionCookie = "encryptedcookies:sessioncookie" // see session.go
)

// GenerateNonce generates a new random string.
func GenerateNonce() []byte {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		panic(fmt.Sprintf("failed to generate nonce: %s", err))
	}
	return nonce
}

// EncryptStateB64 serializes and encrypts `state` used in OpenID protocol.
func EncryptStateB64(aead tink.AEAD, state *encryptedcookiespb.OpenIDState) (string, error) {
	return encryptB64(aead, state, aeadContextState)
}

// DecryptStateB64 decrypts and deserializes `state` used in OpneID protocol.
func DecryptStateB64(aead tink.AEAD, enc string) (*encryptedcookiespb.OpenIDState, error) {
	s := &encryptedcookiespb.OpenIDState{}
	if err := decryptB64(aead, enc, s, aeadContextState); err != nil {
		return nil, err
	}
	return s, nil
}

// EncryptPrivate serializes and encrypts sessionpb.Private proto.
func EncryptPrivate(aead tink.AEAD, private *sessionpb.Private) ([]byte, error) {
	return encrypt(aead, private, aeadContextPrivate)
}

// DecryptPrivate decrypts and deserializes sessionpb.Private proto.
func DecryptPrivate(aead tink.AEAD, enc []byte) (*sessionpb.Private, error) {
	p := &sessionpb.Private{}
	if err := decrypt(aead, enc, p, aeadContextPrivate); err != nil {
		return nil, err
	}
	return p, nil
}

// encrypt serializes and encrypts a proto message.
func encrypt(aead tink.AEAD, msg proto.Message, context string) ([]byte, error) {
	blob, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return aead.Encrypt(blob, []byte(context))
}

// decrypt decrypts and deserializes a proto message.
func decrypt(aead tink.AEAD, enc []byte, msg proto.Message, context string) error {
	blob, err := aead.Decrypt(enc, []byte(context))
	if err != nil {
		return err
	}
	return proto.Unmarshal(blob, msg)
}

// encryptB64 is like encrypt, but base64-encodes the output.
func encryptB64(aead tink.AEAD, msg proto.Message, context string) (string, error) {
	blob, err := encrypt(aead, msg, context)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(blob), nil
}

// decryptB64 is like decrypt, but base64-decodes the input first.
func decryptB64(aead tink.AEAD, enc string, msg proto.Message, context string) error {
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return err
	}
	return decrypt(aead, raw, msg, context)
}
