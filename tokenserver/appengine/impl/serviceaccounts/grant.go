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

package serviceaccounts

import (
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/signing"

	"github.com/luci/luci-go/tokenserver/api"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/tokensigning"
)

// tokenSigningContext is used to make sure grant token is not misused in
// place of some other token.
//
// See SigningContext in utils/tokensigning.Signer.
const tokenSigningContext = "LUCI OAuthTokenGrant v1"

// SignGrant signs and serializes the OAuth grant.
//
// It doesn't do any validation. Assumes the prepared body is valid.
//
// Produces base64 URL-safe token or a transient error.
func SignGrant(c context.Context, signer signing.Signer, tok *tokenserver.OAuthTokenGrantBody) (string, error) {
	s := tokensigning.Signer{
		Signer:         signer,
		SigningContext: tokenSigningContext,
		Wrap: func(w *tokensigning.Unwrapped) proto.Message {
			return &tokenserver.OAuthTokenGrantEnvelope{
				TokenBody:      w.Body,
				Pkcs1Sha256Sig: w.RsaSHA256Sig,
				KeyId:          w.KeyID,
			}
		},
	}
	return s.SignToken(c, tok)
}
