// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package delegation contains low-level API for working with delegation tokens.
//
// Prefer the high-level API in server/auth package, in particular
// `MintDelegationToken` and `auth.GetRPCTransport(ctx, auth.AsUser)`.
package delegation

import (
	"encoding/gob"
	"time"
)

const (
	// HTTPHeaderName is name of HTTP header that carries the token.
	HTTPHeaderName = "X-Delegation-Token-V1"
)

// Token represents serialized and signed delegation token.
type Token struct {
	Token  string    // base64-encoded URL-safe blob with the token
	Expiry time.Time // UTC time when it expires
}

func init() {
	// For the token cache.
	gob.Register(Token{})
}
