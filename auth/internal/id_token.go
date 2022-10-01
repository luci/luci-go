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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// IDTokenClaims contains a *subset* of ID token claims we are interested in.
type IDTokenClaims struct {
	Aud           string `json:"aud"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Exp           int64  `json:"exp"`
	Iss           string `json:"iss"`
	Nonce         string `json:"nonce"`
	Sub           string `json:"sub"`
}

// ParseIDTokenClaims extracts claims of the ID token.
//
// It doesn't validate the signature nor the validity of the claims.
func ParseIDTokenClaims(tok string) (*IDTokenClaims, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("ID token is not a valid JWT - not 3 parts")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("ID token is not a valid JWT - %s", err)
	}
	var out IDTokenClaims
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ID token is not a valid JWT - %s", err)
	}
	return &out, nil
}
