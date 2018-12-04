// Copyright 2018 The LUCI Authors.
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

package serviceaccounts

import (
	"context"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/identity"
	tokenserver "go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
)

// MintOAuthTokenViaGrantRPC implements TokenMinter.MintOAuthTokenViaGrant
// method.
type MintServiceOAuthTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
	Signer signing.Signer

	// Rules returns service account rules to use for the request.
	//
	// In prod it is GlobalRulesCache.Rules.
	Rules func(context.Context) (*Rules, error)

	// MintAccessToken produces an OAuth token for a service account.
	//
	// In prod it is auth.MintAccessTokenForServiceAccount.
	MintAccessToken func(context.Context, auth.MintAccessTokenParams) (*oauth2.Token, error)

	// LogOAuthToken is mocked in tests.
	//
	// In prod it is LogOAuthToken from oauth_token_bigquery_log.go.
	LogOAuthToken func(context.Context, *MintedOAuthTokenInfo) error
}

func (r *MintServiceOAuthTokenRPC) MintServiceOAuthToken(c context.Context, req *minter.MintServiceOAuthTokenRequest) (*minter.MintServiceOAuthTokenResponse, error) {
	return &minter.MintServiceOAuthTokenResponse{}, nil
}

func (r *MintServiceOAuthTokenRPC) validateRequest(c context.Context, req *minter.MintServiceOAuthTokenRequest, caller identity.Identity) (*Rule, error) {
	return nil, nil
}

func (r *MintServiceOAuthTokenRPC) logRequest(c context.Context, req *minter.MintServiceOAuthTokenRequest, caller identity.Identity) {
}

func (r *MintServiceOAuthTokenRPC) checkRequestFormat(req *minter.MintServiceOAuthTokenRequest) error {
	return nil
}

func (r *MintServiceOAuthTokenRPC) checkRules(c context.Context, req *minter.MintServiceOAuthTokenRequest, caller identity.Identity) (*Rule, error) {
	return nil, nil
}

func (r *MintServiceOAuthTokenRPC) mint(c context.Context, p *mintParams) (*minter.MintServiceOAuthTokenResponse, *tokenserver.OAuthTokenGrantBody, error) {
	return nil, nil, nil
}
