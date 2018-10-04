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

package tokensigning

import (
	"context"
	"encoding/base64"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth/signing"
)

// Signer knows how to sign protos and serialize/encode signed result.
type Signer struct {
	// Signer is the actual signer: it knows how to sign blobs.
	Signer signing.Signer

	// SigningContext is prepended to the token blob before it is signed.
	//
	// It exists to avoid cross-protocol attacks, when same key is used to sign
	// different kinds of tokens. An attacker may get a token of kind A, and use
	// it in place of a token of kind B. This may produce unexpected (possibly
	// bad) results, especially for proto-serialized tokens (that all use small
	// integers for message tags).
	//
	// By using different SigningContext strings per token kind we ensure tokens
	// are recognized as correctly signed only when they are used in an
	// appropriate context.
	//
	// SigningContext should be some arbitrary constant string that designates the
	// usage of the token. We actually prepend SigningContext + '\x00' to the
	// token blob.
	//
	// If SigningContext is "", this mechanism is completely skipped.
	SigningContext string

	// Encoding is base64 encoding to use (or RawURLEncoding if nil).
	Encoding *base64.Encoding

	// Wrap takes a blob with token body, the signature and signing key details
	// and returns a proto to serialize/encode and return as the final signed
	// token.
	Wrap func(unwrapped *Unwrapped) proto.Message
}

// SignToken serializes the body, signs it and returns serialized envelope.
//
// Produces base64 URL-safe token or an error (possibly transient).
func (s *Signer) SignToken(c context.Context, body proto.Message) (string, error) {
	info, err := s.Signer.ServiceInfo(c)
	if err != nil {
		return "", transient.Tag.Apply(err)
	}
	blob, err := proto.Marshal(body)
	if err != nil {
		return "", err
	}
	withCtx := prependSigningContext(blob, s.SigningContext)
	keyID, sig, err := s.Signer.SignBytes(c, withCtx)
	if err != nil {
		return "", transient.Tag.Apply(err)
	}
	tok, err := proto.Marshal(s.Wrap(&Unwrapped{
		Body:         blob,
		RsaSHA256Sig: sig,
		SignerID:     info.ServiceAccountName,
		KeyID:        keyID,
	}))
	if err != nil {
		return "", err
	}
	enc := s.Encoding
	if enc == nil {
		enc = base64.RawURLEncoding
	}
	return enc.EncodeToString(tok), nil
}
