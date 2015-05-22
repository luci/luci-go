// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"golang.org/x/oauth2/google"
	"google.golang.org/cloud/compute/metadata"
)

type gceTokenProvider struct {
	oauthTokenProvider

	account string
}

// NewGCETokenProvider returns TokenProvider that knows how to use GCE metadata server.
func NewGCETokenProvider(account string, scopes []string) (TokenProvider, error) {
	// Ensure account has requested scopes.
	availableScopes, err := metadata.Scopes(account)
	if err != nil {
		return nil, err
	}
	for requested := range scopes {
		ok := false
		for available := range availableScopes {
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
	src := google.ComputeTokenSource(p.account)
	tok, err := src.Token()
	if err != nil {
		return nil, err
	}
	return makeToken(tok), nil
}

func (p *gceTokenProvider) RefreshToken(Token) (Token, error) {
	// Minting and refreshing on GCE is the same thing: a call to metadata server.
	return p.MintToken()
}
