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
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
)

// IDToken is verified deserialized ID token.
//
// See https://developers.google.com/identity/protocols/OpenIDConnect.
type IDToken struct {
	Iss           string `json:"iss"`
	AtHash        string `json:"at_hash"`
	EmailVerified bool   `json:"email_verified"`
	Sub           string `json:"sub"`
	Azp           string `json:"azp"`
	Email         string `json:"email"`
	Profile       string `json:"profile"`
	Picture       string `json:"picture"`
	Name          string `json:"name"`
	Aud           string `json:"aud"`
	Iat           int64  `json:"iat"`
	Exp           int64  `json:"exp"`
	Nonce         string `json:"nonce"`
	Hd            string `json:"hd"`
}

const allowedClockSkew = 30 * time.Second

// VerifyIDToken deserializes and verifies the ID token.
//
// This is a fast local operation.
func VerifyIDToken(c context.Context, token string, keys *JSONWebKeySet, issuer, audience string) (*IDToken, error) {
	// See https://developers.google.com/identity/protocols/OpenIDConnect#validatinganidtoken

	body, err := keys.VerifyJWT(token)
	if err != nil {
		return nil, err
	}
	tok := &IDToken{}
	if err := json.Unmarshal(body, tok); err != nil {
		return nil, fmt.Errorf("bad ID token - not JSON - %s", err)
	}

	exp := time.Unix(tok.Exp, 0)
	now := clock.Now(c)

	switch {
	case tok.Iss != issuer && "https://"+tok.Iss != issuer:
		return nil, fmt.Errorf("bad ID token - expecting issuer %q, got %q", issuer, tok.Iss)
	case tok.Aud != audience:
		return nil, fmt.Errorf("bad ID token - expecting audience %q, got %q", audience, tok.Aud)
	case !tok.EmailVerified:
		return nil, fmt.Errorf("bad ID token - the email %q is not verified", tok.Email)
	case tok.Sub == "":
		return nil, fmt.Errorf("bad ID token - the subject is missing")
	case exp.Add(allowedClockSkew).Before(now):
		return nil, fmt.Errorf("bad ID token - expired %s ago", now.Sub(exp))
	}

	return tok, nil
}
