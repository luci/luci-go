// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/cloud/compute/metadata"
)

type gceTokenProvider struct {
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
	return &gceTokenProvider{account: account}, nil
}

func (p *gceTokenProvider) RequiresInteraction() bool {
	return false
}

func (p *gceTokenProvider) CacheSeed() []byte {
	return nil
}

func (p *gceTokenProvider) MintToken() (*oauth2.Token, error) {
	src := google.ComputeTokenSource(p.account)
	return src.Token()
}

func (p *gceTokenProvider) RefreshToken(*oauth2.Token) (*oauth2.Token, error) {
	// Minting and refreshing on GCE is the same thing: a call to metadata server.
	return p.MintToken()
}
