// Copyright 2016 The LUCI Authors.
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
