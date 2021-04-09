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
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GenerateCodeVerifier generates a random string used as a code_verifier in
// the PKCE protocol.
//
// See https://tools.ietf.org/html/rfc7636.
func GenerateCodeVerifier() string {
	blob := make([]byte, 50)
	if _, err := rand.Read(blob); err != nil {
		panic(fmt.Sprintf("failed to generate code verifier: %s", err))
	}
	// Note: there are exactly 64 symbols here. We exclude technically allowed '_'
	// and '~' to simplify random number processing below needed to get fair
	// distribution of probabilities.
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-."
	for i := range blob {
		blob[i] = alphabet[blob[i]&63]
	}
	return string(blob)
}

// DeriveCodeChallenge derives code_challenge from the code_verifier.
func DeriveCodeChallenge(codeVerifier string) string {
	codeVerifierS256 := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(codeVerifierS256[:])
}
