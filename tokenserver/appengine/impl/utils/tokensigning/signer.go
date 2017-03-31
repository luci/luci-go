// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tokensigning

import (
	"encoding/base64"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/server/auth/signing"
)

// Signer knows how to sign protos and serialize/encode signed result.
type Signer struct {
	// Signer is the actual signer: it knows how to sign blobs.
	Signer signing.Signer

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
		return "", errors.WrapTransient(err)
	}
	blob, err := proto.Marshal(body)
	if err != nil {
		return "", err
	}
	keyID, sig, err := s.Signer.SignBytes(c, blob)
	if err != nil {
		return "", errors.WrapTransient(err)
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
