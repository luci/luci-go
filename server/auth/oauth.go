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

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth/identity"
	"go.chromium.org/luci/server/caching"

	"golang.org/x/net/context"
)

// GoogleOAuth2Method implements Method on top of Google's OAuth2 endpoint.
//
// It is useful for development verification (e.g., "dev_appserver") or
// verification in environments without the User API (e.g., Flex).
type GoogleOAuth2Method struct {
	// Scopes is a list of OAuth scopes to check when authenticating the token.
	Scopes []string

	// tokenInfoEndpoint is used in unit test to mock production endpoint.
	tokenInfoEndpoint string
}

// Authenticate implements Method.
func (m *GoogleOAuth2Method) Authenticate(c context.Context, r *http.Request) (*User, error) {
	cfg := getConfig(c)
	if cfg == nil || cfg.AnonymousTransport == nil {
		return nil, ErrNotConfigured
	}

	// Extract the access token from the Authorization header.
	header := r.Header.Get("Authorization")
	if header == "" || len(m.Scopes) == 0 {
		return nil, nil // this method is not applicable
	}

	chunks := strings.SplitN(header, " ", 2)
	if len(chunks) != 2 || (chunks[0] != "OAuth" && chunks[0] != "Bearer") {
		return nil, errors.New("oauth: bad Authorization header")
	}
	accessToken := chunks[1]

	// Check cache.
	user, err := caching.ProcessCache(c).GetOrCreate(c, makeAccessTokenCacheKey(accessToken),
		func() (interface{}, time.Duration, error) {
			// Not cached, or invalid cache. Regenerate and add to cache under lock.
			return m.authenticateAgainstGoogle(c, cfg, accessToken)
		})
	if err != nil {
		return nil, err
	}
	return user.(*User), nil
}

// authenticateAgainstGoogle uses OAuth2 tokeninfo endpoint via URL fetch.
func (m *GoogleOAuth2Method) authenticateAgainstGoogle(c context.Context, cfg *Config, accessToken string) (
	*User, time.Duration, error) {

	// Use the anonymous transport client for OAuth2 verification.
	client := cfg.anonymousClient(c)

	// Fetch an info dict associated with the token.
	logging.Infof(c, "oauth: Querying tokeninfo endpoint")
	tokenInfo, err := googleoauth.GetTokenInfo(c, googleoauth.TokenInfoParams{
		AccessToken: accessToken,
		Client:      client,
		Endpoint:    m.tokenInfoEndpoint,
	})
	if err != nil {
		if err == googleoauth.ErrBadToken {
			return nil, 0, err
		}
		return nil, 0, errors.Annotate(err, "oauth: transient error when validating token").
			Tag(transient.Tag).Err()
	}

	// Verify the token contains a validated email.
	switch {
	case tokenInfo.Email == "":
		return nil, 0, fmt.Errorf("oauth: token is not associated with an email")
	case !tokenInfo.EmailVerified:
		return nil, 0, fmt.Errorf("oauth: email %s is not verified", tokenInfo.Email)
	}
	if tokenInfo.ExpiresIn <= 0 {
		return nil, 0, fmt.Errorf("oauth: 'expires_in' field is not a positive integer")
	}

	// Verify `scopes` is subset of tokenInfo.Scope.
	tokenScopes := map[string]bool{}
	for _, s := range strings.Split(tokenInfo.Scope, " ") {
		tokenScopes[s] = true
	}
	for _, s := range m.Scopes {
		if !tokenScopes[s] {
			return nil, 0, fmt.Errorf("oauth: token doesn't have scope %q", s)
		}
	}

	// Good enough.
	id, err := identity.MakeIdentity("user:" + tokenInfo.Email)
	if err != nil {
		return nil, 0, err
	}

	exp := time.Duration(tokenInfo.ExpiresIn) * time.Second
	u := &User{
		Identity: id,
		Email:    tokenInfo.Email,
		ClientID: tokenInfo.Aud,
	}

	return u, exp, nil
}

type accessTokenCacheKey string

// makeAccessTokenCacheKey creates a cache key for the specified access
// token. To generate this key, we hash the actual access token so that if a
// memory or cache dump ever occurs, the tokens themselves aren't included in
// it.
func makeAccessTokenCacheKey(token string) accessTokenCacheKey {
	h := sha256.Sum256([]byte(token))
	return accessTokenCacheKey(hex.EncodeToString(h[:]))
}
