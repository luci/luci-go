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
	"context"
	"time"

	"go.chromium.org/luci/auth/jwt"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
)

// IDToken is a verified deserialized ID token.
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

	// This section is present only for tokens generated via GCE Metadata server
	// in "full" format.
	Google struct {
		ComputeEngine struct {
			InstanceCreationTimestamp int64  `json:"instance_creation_timestamp"`
			InstanceID                string `json:"instance_id"`
			InstanceName              string `json:"instance_name"`
			ProjectID                 string `json:"project_id"`
			ProjectNumber             int64  `json:"project_number"`
			Zone                      string `json:"zone"`
		} `json:"compute_engine"`
	} `json:"google"`
}

const allowedClockSkew = 30 * time.Second

// VerifyIDToken deserializes and verifies the ID token.
//
// It checks the signature, expiration time and verifies fields `iss` and
// `email_verified`.
//
// It checks `aud` and `sub` are present, but does NOT verify them any further.
// It is the caller's responsibility to do so.
//
// This is a fast local operation.
func VerifyIDToken(ctx context.Context, token string, keys jwt.SignatureVerifier, issuer string) (*IDToken, error) {
	// See https://developers.google.com/identity/protocols/OpenIDConnect#validatinganidtoken

	tok := &IDToken{}
	if err := jwt.VerifyAndDecode(token, tok, keys); err != nil {
		return nil, errors.Fmt("bad ID token: %w", err)
	}

	exp := time.Unix(tok.Exp, 0)
	now := clock.Now(ctx)

	switch {
	case tok.Iss != issuer && "https://"+tok.Iss != issuer:
		return nil, errors.Fmt("bad ID token: expecting issuer %q, got %q", issuer, tok.Iss)
	case exp.Add(allowedClockSkew).Before(now):
		return nil, errors.Fmt("bad ID token: expired %s ago", now.Sub(exp))
	case !tok.EmailVerified:
		return nil, errors.Fmt("bad ID token: the email %q is not verified", tok.Email)
	case tok.Aud == "":
		return nil, errors.New("bad ID token: the audience is missing")
	case tok.Sub == "":
		return nil, errors.New("bad ID token: the subject is missing")
	}

	return tok, nil
}
