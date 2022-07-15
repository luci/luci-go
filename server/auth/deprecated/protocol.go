// Copyright 2015 The LUCI Authors.
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

package deprecated

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/internal"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/tokens"
)

// Note: this file is a part of deprecated CookieAuthMethod implementation.

// openIDStateToken is used to generate `state` parameter used in OpenID flow to
// pass state between our app and authentication backend.
var openIDStateToken = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: 30 * time.Minute,
	SecretKey:  "openid_state_token",
	Version:    1,
}

// authenticationURI returns an URI to redirect a user to in order to
// authenticate via OpenID.
//
// This is step 1 of the authentication flow. Generate authentication URL and
// redirect user's browser to it. After consent screen, redirect_uri will be
// called (via user's browser) with `state` and authorization code passed to it,
// eventually resulting in a call to 'handle_authorization_code'.
func authenticationURI(ctx context.Context, cfg *Settings, state map[string]string) (string, error) {
	if cfg.ClientID == "" || cfg.RedirectURI == "" || cfg.DiscoveryURL == "" {
		return "", ErrNotConfigured
	}

	// Grab authorization URL from discovery doc.
	discovery, err := openid.FetchDiscoveryDoc(ctx, cfg.DiscoveryURL)
	if err != nil {
		return "", err
	}
	if discovery.AuthorizationEndpoint == "" {
		return "", errors.New("openid: bad discovery doc, empty authorization_endpoint")
	}

	// Wrap state into HMAC-protected token.
	stateTok, err := openIDStateToken.Generate(ctx, nil, state, 0)
	if err != nil {
		return "", err
	}

	// Generate final URL.
	v := url.Values{}
	v.Set("client_id", cfg.ClientID)
	v.Set("redirect_uri", cfg.RedirectURI)
	v.Set("response_type", "code")
	v.Set("scope", "openid email profile")
	v.Set("prompt", "select_account")
	v.Set("state", stateTok)
	return discovery.AuthorizationEndpoint + "?" + v.Encode(), nil
}

// validateStateToken validates 'state' token passed to redirect_uri. Returns
// whatever `state` was passed to authenticationURI.
func validateStateToken(ctx context.Context, stateTok string) (map[string]string, error) {
	return openIDStateToken.Validate(ctx, stateTok, nil)
}

// handleAuthorizationCode exchange `code` for user ID token and user profile.
func handleAuthorizationCode(ctx context.Context, cfg *Settings, code string) (uid string, u *auth.User, err error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURI == "" {
		return "", nil, ErrNotConfigured
	}

	// Validate the discover doc has necessary fields to proceed.
	discovery, err := openid.FetchDiscoveryDoc(ctx, cfg.DiscoveryURL)
	switch {
	case err != nil:
		return "", nil, err
	case discovery.TokenEndpoint == "":
		return "", nil, errors.New("openid: bad discovery doc, empty token_endpoint")
	}

	// Prepare a request to exchange authorization code for the ID token.
	v := url.Values{}
	v.Set("code", code)
	v.Set("client_id", cfg.ClientID)
	v.Set("client_secret", cfg.ClientSecret)
	v.Set("redirect_uri", cfg.RedirectURI)
	v.Set("grant_type", "authorization_code")
	payload := v.Encode()

	// Send POST to the token endpoint with URL-encoded parameters to get back the
	// ID token. There's more stuff in the reply, we don't need it.
	var token struct {
		IDToken string `json:"id_token"`
	}
	req := internal.Request{
		Method: "POST",
		URL:    discovery.TokenEndpoint,
		Body:   []byte(payload),
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
		Out: &token,
	}
	if err := req.Do(ctx); err != nil {
		return "", nil, err
	}

	// Unpack the ID token to grab the user information from it.
	tok, user, err := openid.UserFromIDToken(ctx, token.IDToken, discovery)
	if err != nil {
		return "", nil, err
	}
	// Make sure the token was created via the expected OAuth client.
	if tok.Aud != cfg.ClientID {
		return "", nil, fmt.Errorf("bad ID token - expecting audience %q, got %q", cfg.ClientID, tok.Aud)
	}
	return tok.Sub, user, nil
}
