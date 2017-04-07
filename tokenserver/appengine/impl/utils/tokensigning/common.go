// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package tokensigning implements utilities for RSA-signing of proto messages.
package tokensigning

import "time"

// Unwrapped carries a serialized token proto and its signature.
//
// It is then converted into some concrete proto, serialized, base64-encoded and
// returned to the clients.
//
// 'Wrap' may use Body, RsaSHA256Sig, SignerID and KeyID fields.
// 'Unwrap' must initialize Body, RsaSHA256Sig, KeyID.
type Unwrapped struct {
	Body         []byte // serialized proto that was signed
	RsaSHA256Sig []byte // the actual signature
	SignerID     string // service account email that owns the signing key
	KeyID        string // identifier of the signing key
}

// Lifespan is a time interval when some token is valid.
type Lifespan struct {
	NotBefore time.Time
	NotAfter  time.Time
}

// prependSigningContext prepends '<ctx>\x00' to the blob, if ctx != "".
//
// See SigningContext in Signer for more info.
func prependSigningContext(blob []byte, ctx string) []byte {
	if ctx == "" {
		return blob
	}
	b := make([]byte, len(blob)+len(ctx)+1)
	copy(b, ctx)
	copy(b[len(ctx)+1:], blob)
	return b
}
