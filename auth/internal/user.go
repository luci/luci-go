// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
)

type userAuthTokenProvider struct {
	oauthTokenProvider

	ctx    context.Context
	config *oauth2.Config
}

// NewUserAuthTokenProvider returns TokenProvider that can perform 3-legged
// OAuth flow involving interaction with a user.
func NewUserAuthTokenProvider(ctx context.Context, clientID, clientSecret string, scopes []string) (TokenProvider, error) {
	return &userAuthTokenProvider{
		oauthTokenProvider: oauthTokenProvider{
			interactive: true,
			tokenFlavor: "user",
		},
		ctx: ctx,
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
	}, nil
}

func (p *userAuthTokenProvider) MintToken() (Token, error) {
	// Grab the authorization code by redirecting a user to a consent screen.
	url := p.config.AuthCodeURL("", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Printf("Visit the URL to get authorization code.\n\n%s\n\n", url)
	fmt.Printf("Authorization code: ")
	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, err
	}
	// Exchange it for a token.
	tok, err := p.config.Exchange(p.ctx, code)
	if err != nil {
		return nil, err
	}
	return makeToken(tok), nil
}

func (p *userAuthTokenProvider) RefreshToken(tok Token) (Token, error) {
	// Clear expiration time to force token refresh. Do not use 0 since it means
	// that token never expires.
	t := extractOAuthToken(tok)
	t.Expiry = time.Unix(1, 0)
	src := p.config.TokenSource(p.ctx, &t)
	newTok, err := src.Token()
	if err != nil {
		return nil, err
	}
	return makeToken(newTok), nil
}
