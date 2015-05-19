// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"time"

	"infra/libs/gce"
)

type gceTokenProvider struct {
	oauthTokenProvider

	account string
}

// NewGCETokenProvider returns TokenProvider that knows how to use GCE metadata server.
func NewGCETokenProvider(account string, scopes []string) (TokenProvider, error) {
	// Ensure account has requested scopes.
	acc, err := gce.GetServiceAccount(account)
	if err != nil {
		return nil, err
	}
	for requested := range scopes {
		ok := false
		for available := range acc.Scopes {
			if requested == available {
				ok = true
				break
			}
		}
		if !ok {
			return nil, ErrInsufficientAccess
		}
	}
	return &gceTokenProvider{
		oauthTokenProvider: oauthTokenProvider{
			interactive: false,
			tokenFlavor: "gce",
		},
		account: account,
	}, nil
}

func (p *gceTokenProvider) MintToken() (Token, error) {
	tokenData, err := gce.GetAccessToken(p.account)
	if err != nil {
		return nil, err
	}
	tok := &tokenImpl{}
	tok.AccessToken = tokenData.AccessToken
	tok.Expiry = time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second)
	return tok, nil
}

func (p *gceTokenProvider) RefreshToken(Token) (Token, error) {
	// Minting and refreshing on GCE is the same thing: a call to metadata server.
	return p.MintToken()
}
