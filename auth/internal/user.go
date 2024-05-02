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
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

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
			Endpoint:     google.Endpoint,
			RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
			Scopes:       scopes,
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

func (p *userAuthTokenProvider) Email() string {
	// We don't know the email before user logs in.
	return UnknownEmail
}

func (p *userAuthTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *userAuthTokenProvider) MintToken(ctx context.Context, base *Token) (*Token, error) {
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

	// Exchange it for refresh, access and (possibly) ID tokens.
	tok, err := p.config.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	return processProviderReply(ctx, tok, "")
}

func (p *userAuthTokenProvider) RefreshToken(ctx context.Context, prev, base *Token) (*Token, error) {
	return refreshToken(ctx, prev, base, p.config)
}

func refreshToken(ctx context.Context, prev, base *Token, cfg *oauth2.Config) (*Token, error) {
	// Clear expiration time to force token refresh. Do not use 0 since it means
	// that token never expires.
	t := prev.Token
	t.Expiry = time.Unix(1, 0)
	switch newTok, err := grabToken(cfg.TokenSource(ctx, &t)); {
	case err == nil:
		return processProviderReply(ctx, newTok, prev.Email)
	case transient.Tag.In(err):
		logging.Warningf(ctx, "Transient error when refreshing the token - %s", err)
		return nil, err
	default:
		logging.Warningf(ctx, "Bad refresh token - %s", err)
		return nil, ErrBadRefreshToken
	}
}

// processProviderReply transforms oauth2.Token into Token by extracting some
// useful information from it.
//
// May make an RPC to the token info endpoint.
func processProviderReply(ctx context.Context, tok *oauth2.Token, email string) (*Token, error) {
	// If have the ID token, parse its payload to see the expiry and the email.
	// Note that we don't verify the signature. We just got the token from the
	// provider we trust.
	var claims *IDTokenClaims
	var idToken string
	var err error
	if idToken, _ = tok.Extra("id_token").(string); idToken != "" {
		if claims, err = ParseIDTokenClaims(idToken); err != nil {
			return nil, err
		}
	} else {
		idToken = NoIDToken
	}

	// ID token has the freshest email.
	if claims != nil && claims.EmailVerified && claims.Email != "" {
		email = claims.Email
	} else if email == "" {
		// If we still don't know the email associated with the credentials, make
		// an RPC to the token info endpoint to get it.
		if email, err = grabEmail(ctx, tok); err != nil {
			return nil, err
		}
	}

	// We rely on `tok` expiry to know when to refresh both the access and ID
	// tokens. Usually they have roughly the same expiry. Check this.
	if claims != nil {
		idTokenExpiry := time.Unix(claims.Exp, 0)
		delta := idTokenExpiry.Sub(tok.Expiry)
		if delta < 0 {
			delta = -delta
		}
		if delta > time.Minute {
			logging.Warningf(ctx, "The ID token and access tokens have unexpectedly large discrepancy in expiration times: %v", delta)
		}
		if idTokenExpiry.Before(tok.Expiry) {
			tok.Expiry = idTokenExpiry
		}
	}

	return &Token{
		Token:   *tok,
		IDToken: idToken,
		Email:   email,
	}, nil
}

// grabEmail fetches an email associated with the given token.
//
// May return (NoEmail, nil) if the token can't be resolved into an email.
func grabEmail(ctx context.Context, tok *oauth2.Token) (string, error) {
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
