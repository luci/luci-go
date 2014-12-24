// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"fmt"
	"net/http"
	"strings"

	"code.google.com/p/goauth2/oauth"
)

type userAuthTokenProvider struct {
	oauthTokenProvider

	config *oauth.Config
}

// NewUserAuthTokenProvider returns TokenProvider that can perform 3-legged
// OAuth flow involving interaction with a user.
func NewUserAuthTokenProvider(clientID, clientSecret string, scopes []string) (TokenProvider, error) {
	return &userAuthTokenProvider{
		oauthTokenProvider: oauthTokenProvider{
			interactive: true,
			tokenFlavor: "user",
		},
		config: &oauth.Config{
			ClientId:       clientID,
			ClientSecret:   clientSecret,
			Scope:          strings.Join(scopes, " "),
			AuthURL:        "https://accounts.google.com/o/oauth2/auth",
			TokenURL:       "https://accounts.google.com/o/oauth2/token",
			RedirectURL:    "urn:ietf:wg:oauth:2.0:oob",
			AccessType:     "offline",
			ApprovalPrompt: "force",
		},
	}, nil
}

func (p *userAuthTokenProvider) MintToken(rt http.RoundTripper) (Token, error) {
	// Grab the authorization code by redirecting a user to a consent screen.
	url := p.config.AuthCodeURL("")
	fmt.Printf("Visit the URL to get authorization code.\n\n%s\n\n", url)
	fmt.Printf("Authorization code: ")
	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, err
	}

	// Exchange it for a token.
	transport := &oauth.Transport{
		Config:    p.config,
		Transport: rt,
	}
	tok, err := transport.Exchange(code)
	if err != nil {
		return nil, err
	}
	return makeToken(tok), nil
}

func (p *userAuthTokenProvider) RefreshToken(tok Token, rt http.RoundTripper) (Token, error) {
	copied := extractOAuthToken(tok)
	transport := &oauth.Transport{
		Config:    p.config,
		Token:     &copied,
		Transport: rt,
	}
	err := transport.Refresh()
	if err != nil {
		return nil, err
	}
	return makeToken(transport.Token), nil
}
