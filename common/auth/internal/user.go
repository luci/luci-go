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

package internal

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
)

type userAuthTokenProvider struct {
	config   *oauth2.Config
	cacheKey CacheKey
}

// NewUserAuthTokenProvider returns TokenProvider that can perform 3-legged
// OAuth flow involving interaction with a user.
func NewUserAuthTokenProvider(ctx context.Context, clientID, clientSecret string, scopes []string) (TokenProvider, error) {
	return &userAuthTokenProvider{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/auth",
				TokenURL: "https://accounts.google.com/o/oauth2/token",
			},
			RedirectURL: "urn:ietf:wg:oauth:2.0:oob",
			Scopes:      scopes,
		},
		cacheKey: CacheKey{
			Key:    fmt.Sprintf("user/%s", clientID),
			Scopes: scopes,
		},
	}, nil
}

func (p *userAuthTokenProvider) RequiresInteraction() bool {
	return true
}

func (p *userAuthTokenProvider) Lightweight() bool {
	return false
}

func (p *userAuthTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *userAuthTokenProvider) MintToken(ctx context.Context, base *Token) (*Token, error) {
	if p.config.ClientID == "" || p.config.ClientSecret == "" {
		return nil, fmt.Errorf("OAuth client is not set, can't use 3-legged login flow")
	}

	// The list of scopes is displayed on the consent page as well, but show it
	// in the terminal too, for clarity.
	fmt.Println("Getting a refresh token with following OAuth scopes:")
	for _, scope := range p.config.Scopes {
		fmt.Printf("  * %s\n", scope)
	}
	fmt.Println()

	// Grab the authorization code by redirecting a user to a consent screen.
	url := p.config.AuthCodeURL("", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Printf("Visit the following URL to get the authorization code and copy-paste it below.\n\n%s\n\n", url)
	fmt.Printf("Authorization code: ")
	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, err
	}
	fmt.Println()

	// Exchange it for a token.
	tok, err := p.config.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}

	// Grab an email associated with the token, if possible. May return NoEmail.
	email, err := p.grabEmail(ctx, tok)
	if err != nil {
		return nil, err
	}

	return &Token{
		Token: *tok,
		Email: email,
	}, nil
}

func (p *userAuthTokenProvider) RefreshToken(ctx context.Context, prev, base *Token) (*Token, error) {
	// Clear expiration time to force token refresh. Do not use 0 since it means
	// that token never expires.
	t := prev.Token
	t.Expiry = time.Unix(1, 0)
	switch newTok, err := grabToken(p.config.TokenSource(ctx, &t)); {
	case err == nil:
		// If we didn't have an email before, grab it now. This is important to
		// "upgrade" existing cached tokens to include email.
		email := prev.Email
		if email == UnknownEmail {
			var err error
			email, err = p.grabEmail(ctx, newTok)
			if err != nil {
				return nil, err
			}
		}
		return &Token{
			Token: *newTok,
			Email: email,
		}, nil
	case transient.Tag.In(err):
		logging.Warningf(ctx, "Transient error when refreshing the token - %s", err)
		return nil, err
	default:
		logging.Warningf(ctx, "Bad refresh token - %s", err)
		return nil, ErrBadRefreshToken
	}
}

// grabEmail fetches an email associated with the given token.
//
// May return (NoEmail, nil) if the token can't be resolved into an email.
func (p *userAuthTokenProvider) grabEmail(ctx context.Context, tok *oauth2.Token) (string, error) {
	info, err := googleoauth.GetTokenInfo(ctx, googleoauth.TokenInfoParams{
		AccessToken: tok.AccessToken,
	})
	if err != nil {
		return "", err
	}
	if info.Email == "" {
		return NoEmail, nil
	}
	return info.Email, nil
}
