// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"code.google.com/p/goauth2/oauth/jwt"
)

type serviceAccountTokenProvider struct {
	oauthTokenProvider

	jwtToken *jwt.Token
}

// NewServiceAccountTokenProvider returns TokenProvider that supports service accounts.
func NewServiceAccountTokenProvider(credsPath string, scopes []string) (TokenProvider, error) {
	buf, err := ioutil.ReadFile(credsPath)
	if err != nil {
		return nil, err
	}
	var key struct {
		Email      string `json:"client_email"`
		PrivateKey string `json:"private_key"`
	}
	if err = json.Unmarshal(buf, &key); err != nil {
		return nil, err
	}
	return &serviceAccountTokenProvider{
		oauthTokenProvider: oauthTokenProvider{
			interactive: false,
			tokenFlavor: "service_account",
		},
		jwtToken: jwt.NewToken(key.Email, strings.Join(scopes, " "), []byte(key.PrivateKey)),
	}, nil
}

func (p *serviceAccountTokenProvider) MintToken(rt http.RoundTripper) (Token, error) {
	tok, err := p.jwtToken.Assert(&http.Client{Transport: rt})
	if err != nil {
		return nil, err
	}
	return makeToken(tok), nil
}

func (p *serviceAccountTokenProvider) RefreshToken(_ Token, rt http.RoundTripper) (Token, error) {
	// JWT tokens are self sufficient, there's no need for refresh_token. Minting
	// a token and "refreshing" it is a same thing.
	return p.MintToken(rt)
}
