// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"infra/libs/gce"
)

type gceTokenProvider struct {
	oauthTokenProvider

	account string
}

// NewGCETokenProvider returns TokenProvider that knows how to use GCE metadata server.
func NewGCETokenProvider(account string, scopes []string) (TokenProvider, error) {
	// TODO(vadimsh): Check via metadata server that account has access to requested scopes.
	return &gceTokenProvider{
		oauthTokenProvider: oauthTokenProvider{
			interactive: false,
			tokenFlavor: "gce",
		},
		account: account,
	}, nil
}

func (p *gceTokenProvider) MintToken(_ http.RoundTripper) (Token, error) {
	// Grab.
	uri := fmt.Sprintf("/computeMetadata/v1/instance/service-accounts/%s/token", p.account)
	resp, err := gce.QueryGCEMetadata(uri, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Deserialize.
	var tokenData struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	err = json.NewDecoder(resp.Body).Decode(&tokenData)
	if err != nil {
		return nil, err
	}

	// Convert to tokenImpl.
	tok := &tokenImpl{}
	tok.AccessToken = tokenData.AccessToken
	tok.Expiry = time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second)
	return tok, nil
}

func (p *gceTokenProvider) RefreshToken(_ Token, rt http.RoundTripper) (Token, error) {
	// Minting and refreshing on GCE is the same thing: a call to metadata server.
	return p.MintToken(rt)
}
