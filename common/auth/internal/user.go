// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
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

func (p *userAuthTokenProvider) MintToken(ctx context.Context, base *oauth2.Token) (*oauth2.Token, error) {
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
	return p.config.Exchange(ctx, code)
}

func (p *userAuthTokenProvider) RefreshToken(ctx context.Context, prev, base *oauth2.Token) (*oauth2.Token, error) {
	// Clear expiration time to force token refresh. Do not use 0 since it means
	// that token never expires.
	t := *prev
	t.Expiry = time.Unix(1, 0)
	switch newTok, err := grabToken(p.config.TokenSource(ctx, &t)); {
	case err == nil:
		return newTok, nil
	case transient.Tag.In(err):
		logging.Warningf(ctx, "Transient error when refreshing the token - %s", err)
		return nil, err
	default:
		logging.Warningf(ctx, "Bad refresh token - %s", err)
		return nil, ErrBadRefreshToken
	}
}
