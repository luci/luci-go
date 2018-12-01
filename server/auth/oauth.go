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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"
)

// ErrBadOAuthToken is returned by GoogleOAuth2Method if the access token it
// checks is bad.
var ErrBadOAuthToken = errors.New("oauth: bad access token")

// SHA256(access token) => JSON-marshalled User or "" if the token is invalid.
var oauthChecksCache = layered.Cache{
	ProcessLRUCache: caching.RegisterLRUCache(4096),
	GlobalNamespace: "oauth_token_checks_v1",
	Marshal: func(item interface{}) ([]byte, error) {
		if item == nil {
			return nil, nil // not a valid token
		}
		return json.Marshal(item.(*User))
	},
	Unmarshal: func(blob []byte) (interface{}, error) {
		if len(blob) == 0 {
			return nil, nil // not a valid token
		}
		u := &User{}
		if err := json.Unmarshal(blob, u); err != nil {
			return nil, err
		}
		return u, nil
	},
}

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

var _ UserCredentialsGetter = (*GoogleOAuth2Method)(nil)

// Authenticate implements Method.
func (m *GoogleOAuth2Method) Authenticate(c context.Context, r *http.Request) (user *User, err error) {
	cfg := getConfig(c)
	if cfg == nil || cfg.AnonymousTransport == nil {
		return nil, ErrNotConfigured
	}

	// Extract the access token from the Authorization header.
	header := r.Header.Get("Authorization")
	if header == "" || len(m.Scopes) == 0 {
		return nil, nil // this method is not applicable
	}
	accessToken, err := accessTokenFromHeader(header)
	if err != nil {
		return nil, err
	}

	// Store only the token hash in the cache, so that if a memory or cache dump
	// ever occurs, the tokens themselves aren't included in it.
	h := sha256.Sum256([]byte(accessToken))
	cacheKey := hex.EncodeToString(h[:])

	// Verify the token or grab a result of previous verification. We cache both
	// good and bad tokens. Note that bad token can't turn into good with passage
	// of time, so its OK to cache it.
	//
	// TODO(vadimsh): Strictly speaking we need to store bad tokens in a separate
	// cache, so a flood of bad tokens don't evict good tokens from the process
	// cache.
	cached, err := oauthChecksCache.GetOrCreate(c, cacheKey, func() (interface{}, time.Duration, error) {
		switch user, exp, err := m.authenticateAgainstGoogle(c, cfg, accessToken); {
		case transient.Tag.In(err):
			logging.WithError(err).Errorf(c, "oauth: Transient error when checking the token")
			return nil, 0, err
		case err != nil:
			// Cache the fact that the token is bad for 30 min. No need to recheck it
			// again just to find out it is (still) bad.
			logging.WithError(err).Errorf(c, "oauth: Caching bad access token SHA256=%q", cacheKey)
			return nil, 30 * time.Minute, nil
		default:
			return user, exp, nil
		}
	})

	switch {
	case err != nil:
		return nil, err // can only be a transient error
	case cached == nil:
		logging.Errorf(c, "oauth: Bad access token SHA256=%q", cacheKey)
		return nil, ErrBadOAuthToken
	}

	user = cached.(*User)
	return
}

// GetUserCredentials implements UserCredentialsGetter.
func (m *GoogleOAuth2Method) GetUserCredentials(c context.Context, r *http.Request) (*oauth2.Token, error) {
	accessToken, err := accessTokenFromHeader(r.Header.Get("Authorization"))
	if err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken: accessToken,
		TokenType:   "Bearer",
	}, nil
}

var errBadAuthHeader = errors.New("oauth: bad Authorization header")

func accessTokenFromHeader(header string) (string, error) {
	chunks := strings.SplitN(header, " ", 2)
	if len(chunks) != 2 || (chunks[0] != "OAuth" && chunks[0] != "Bearer") {
		return "", errBadAuthHeader
	}
	return chunks[1], nil
}

// authenticateAgainstGoogle uses OAuth2 tokeninfo endpoint via URL fetch.
//
// May return transient errors.
//
// TODO(vadimsh): Reduce timeout and add retries on transient errors (they do
// happen).
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
